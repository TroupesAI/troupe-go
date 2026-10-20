package troupe

import (
	"archive/zip"
	"cmp"
	"errors"
	"io"
	"io/fs"
	"slices"
	"strings"
	"sync"
)

// Package is a package: its bundle, its sessions and its other files (Format
// 12.3).
type Package struct {
	Bundle *Bundle
	// BundleJSON is bundle.json as written, which a writer rewrites to keep
	// every ext unchanged (Format 13).
	BundleJSON []byte
	// Sessions are the paths of the package's sessions, sorted, such as
	// "sessions/a.troupe-session.jsonl". ReadSessions reads them.
	Sessions []string
	// Files are the paths of the package's files other than bundle.json and
	// its sessions, sorted. They include files nothing references, which a
	// writer keeps (Format 12.3).
	Files []string
	files map[string]*zip.File
	diags []Diagnostic // ReadPackage's
}

// Open opens the file of the package at path, such as one of Files, to read
// its content as a stream. It reads from the io.ReaderAt given to
// ReadPackage, which must stay unchanged, and is safe for concurrent use. Its
// error is an fs.ErrNotExist when path names no file of the package.
func (p *Package) Open(path string) (io.ReadCloser, error) {
	f := p.files[path]
	if f == nil {
		return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
	}
	return f.Open()
}

// ReadPackage reads a package, the ZIP archive of size bytes in r, and its
// bundle, as a bundle reader does, leaving its sessions to ReadSessions
// (Format 12.3, 15.2). It reads every entry, so data that can't be read is
// PARSE wherever it is. Its results are those of ReadBundle, for the package
// as a bundle reader reads it: the package is nil when the archive fails PARSE, or its
// bundle.json PARSE or UNSUPPORTED_VERSION, and its diagnostics are then only
// those. Package.Open and Package.ReadSessions read r again.
func ReadPackage(r io.ReaderAt, size int64) (*Package, []Diagnostic, error) {
	src := &source{r: r}
	var rd reader
	p := rd.pkg(src, size)
	if src.err != nil {
		return nil, nil, src.err
	}
	diags := rd.diagnostics()
	if p != nil {
		p.diags = slices.Clone(diags)
	}
	return p, diags, nil
}

// ReadSessions reads the package's sessions, by path, as a session reader
// does (Format 15.2). Its diagnostics are the package's at that level,
// ReadPackage's and the sessions', so they replace ReadPackage's. When any
// session fails PARSE or UNSUPPORTED_VERSION, the package fails whole: the
// sessions are nil, and the diagnostics only those (Format 15.3). The error
// is r's, when reading it fails.
func (p *Package) ReadSessions() (map[string]*Session, []Diagnostic, error) {
	rd := reader{files: map[string]bool{}, diags: slices.Clone(p.diags)}
	for name := range p.files {
		rd.files[name] = true
	}
	sessions := map[string]*Session{}
	for name, f := range p.files {
		if !isSession(name) {
			continue
		}
		r, err := f.Open()
		if err != nil {
			return nil, nil, err
		}
		rd.file = name
		s, err := rd.session(r)
		if err := cmp.Or(err, r.Close()); err != nil {
			return nil, nil, err
		}
		if s != nil {
			sessions[name] = s
		}
	}
	if rd.unread() {
		sessions = nil
	}
	return sessions, rd.diagnostics(), nil
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
	files := map[string]*zip.File{}
	names := map[string]bool{}
	for _, f := range zr.File {
		folded := asciiLower(f.Name)
		if !validEntry(f) || names[folded] {
			rd.reportEntry(f.Name)
		}
		names[folded] = true
		if isPackageFile(f.Name) {
			files[f.Name] = f
		}
	}
	if files["bundle.json"] == nil {
		rd.report(Diagnostic{Code: CodeParse})
	}
	if len(rd.diags) > 0 {
		return nil
	}
	rd.files = map[string]bool{}
	for name := range files {
		rd.files[name] = true
	}

	p := &Package{files: files}
	for _, f := range zr.File {
		switch {
		case files[f.Name] == nil || f.Name == "bundle.json":
		case isSession(f.Name):
			p.Sessions = append(p.Sessions, f.Name)
		default:
			p.Files = append(p.Files, f.Name)
		}
		if err := rd.entry(f, p); err != nil {
			rd.reportEntry(f.Name)
			return nil
		}
	}
	if rd.unread() {
		return nil
	}
	slices.Sort(p.Sessions)
	slices.Sort(p.Files)
	return p
}

// entry reads one ZIP entry to its end, as the package's bundle when it is
// bundle.json. Its error is for data that can't be read.
func (rd *reader) entry(f *zip.File, p *Package) (err error) {
	r, err := f.Open()
	if err != nil {
		return err
	}
	defer func() { err = cmp.Or(err, r.Close()) }()
	if f.Name == "bundle.json" {
		rd.file = f.Name
		if p.Bundle, p.BundleJSON, err = rd.bundle(r); err != nil {
			return err
		}
	}
	// Bounded: the entry's size counts toward the unpacked size checked in pkg.
	_, err = io.Copy(io.Discard, r) //nolint:gosec // G110: see above
	return err
}

// isSession reports whether a file of a package is one of its sessions
// (Format 12.3).
func isSession(name string) bool {
	return strings.HasPrefix(name, "sessions/") && strings.HasSuffix(name, ".troupe-session.jsonl")
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

// asciiLower returns name with A to Z as a to z, byte by byte, so a name that
// isn't UTF-8 keeps its other bytes (Format 12.3).
func asciiLower(name string) string {
	b := []byte(name)
	for i, c := range b {
		if 'A' <= c && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}

// source records the first error of a caller's io.ReaderAt, so a failure to
// read is told apart from an archive that isn't valid.
type source struct {
	r    io.ReaderAt
	once sync.Once // Package.Open reads concurrently
	err  error
}

func (s *source) ReadAt(p []byte, off int64) (int, error) {
	n, err := s.r.ReadAt(p, off)
	if err != nil && !errors.Is(err, io.EOF) {
		s.once.Do(func() { s.err = err })
	}
	return n, err
}
