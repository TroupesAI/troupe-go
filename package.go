package troupe

import (
	"archive/zip"
	"cmp"
	"errors"
	"io"
	"io/fs"
	"strings"
)

// Package is a package's bundle and sessions (Format 12.3).
type Package struct {
	Bundle *Bundle
	// Sessions are the package's sessions that read without PARSE or
	// UNSUPPORTED_VERSION, by path, such as "sessions/a.troupe-session.jsonl".
	Sessions map[string]*Session
}

// ReadPackage reads a package, the ZIP archive of size bytes in r, and its
// bundle and sessions (Format 12.3). It reads every entry, so data that can't
// be read is PARSE wherever it is. Its results are those of ReadBundle: the
// package is nil when the archive fails PARSE, or its bundle.json fails PARSE
// or UNSUPPORTED_VERSION, and leaves out any session that does.
func ReadPackage(r io.ReaderAt, size int64) (*Package, []Diagnostic, error) {
	src := &source{r: r}
	var rd reader
	p := rd.pkg(src, size)
	if src.err != nil {
		return nil, nil, src.err
	}
	return p, sortDiagnostics(rd.diags), nil
}

// pkg reads the package in src, stopping when src fails.
func (rd *reader) pkg(src *source, size int64) *Package {
	zr, err := zip.NewReader(src, size)
	// Format 12.3's rules on names are checked below, whatever GODEBUG says.
	if err != nil && !errors.Is(err, zip.ErrInsecurePath) {
		rd.report(Diagnostic{Code: CodeParse})
		return nil
	}
	if len(zr.File) > maxZipEntries {
		rd.report(Diagnostic{Code: CodeParse})
		return nil
	}
	// archive/zip fails an entry whose data doesn't match the size its
	// header gives, so the sizes bound what decompressing produces.
	var unpacked uint64
	for _, f := range zr.File {
		unpacked += min(f.UncompressedSize64, maxUnpackedSize+1)
	}
	if unpacked > maxUnpackedSize {
		rd.report(Diagnostic{Code: CodeParse})
		return nil
	}
	files := map[string]bool{}
	names := map[string]bool{}
	for _, f := range zr.File {
		folded := strings.Map(asciiLower, f.Name)
		if !validEntry(f) || names[folded] {
			rd.reportEntry(f.Name)
		}
		names[folded] = true
		if isPackageFile(f.Name) {
			files[f.Name] = true
		}
	}
	if !files["bundle.json"] {
		rd.report(Diagnostic{Code: CodeParse})
	}
	if len(rd.diags) > 0 {
		return nil
	}
	rd.files = func(path string) bool { return files[path] }

	p := &Package{Sessions: map[string]*Session{}}
	for _, f := range zr.File {
		isSession := files[f.Name] && strings.HasPrefix(f.Name, "sessions/") && strings.HasSuffix(f.Name, ".troupe-session.jsonl")
		if err := rd.entry(f, isSession, p); err != nil {
			rd.reportEntry(f.Name)
			// Only a session can fail and leave the package readable.
			if src.err != nil || !isSession {
				return nil
			}
		}
	}
	if p.Bundle == nil {
		return nil
	}
	return p
}

// entry reads one ZIP entry to its end, as the package's bundle or one of its
// sessions when it is one. Its error is for data that can't be read.
func (rd *reader) entry(f *zip.File, isSession bool, p *Package) (err error) {
	r, err := f.Open()
	if err != nil {
		return err
	}
	defer func() { err = cmp.Or(err, r.Close()) }()
	rd.file = f.Name
	defer func() { rd.file = "" }()
	switch {
	case f.Name == "bundle.json":
		p.Bundle, err = rd.bundle(r)
	case isSession:
		var s *Session
		if s, err = rd.session(r); s != nil {
			p.Sessions[f.Name] = s
		}
	}
	if err != nil {
		return err
	}
	// Bounded: the entry's size counts toward the unpacked size checked in pkg.
	_, err = io.Copy(io.Discard, r) //nolint:gosec // G110: see above
	return err
}

// validEntry reports whether a ZIP entry follows Format 12.3: no absolute path,
// .. segment or backslash, no symbolic link or encryption, and stored or
// deflated.
func validEntry(f *zip.File) bool {
	name := f.Name
	absolute := strings.HasPrefix(name, "/") || len(name) >= 2 && name[1] == ':'
	return !absolute &&
		!strings.Contains(name, `\`) &&
		!strings.Contains("/"+name+"/", "/../") &&
		f.Mode()&fs.ModeSymlink == 0 &&
		f.Flags&0x1 == 0 && // encrypted
		(f.Method == zip.Store || f.Method == zip.Deflate)
}

// reportEntry reports PARSE for a ZIP entry, located by its name only when
// that is a package path, since other names may hold anything.
func (rd *reader) reportEntry(name string) {
	d := Diagnostic{Code: CodeParse}
	if isPackageFile(name) {
		d.File = name
	}
	rd.diags = append(rd.diags, d)
}

func asciiLower(r rune) rune {
	if 'A' <= r && r <= 'Z' {
		return r + ('a' - 'A')
	}
	return r
}

// source records the first error of a caller's io.ReaderAt, so a failure to
// read is told apart from an archive that isn't valid.
type source struct {
	r   io.ReaderAt
	err error
}

func (s *source) ReadAt(p []byte, off int64) (int, error) {
	n, err := s.r.ReadAt(p, off)
	if err != nil && !errors.Is(err, io.EOF) && s.err == nil {
		s.err = err
	}
	return n, err
}
