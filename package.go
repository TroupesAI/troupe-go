package troupe

import (
	"archive/zip"
	"cmp"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"slices"
	"strings"
	"sync"
)

// Package is a package: its story, its sessions and its other files (Format
// 12.3).
type Package struct {
	Story *Story
	// StoryJSON is story.json as written, which a writer rewrites to keep
	// the ext of everything it keeps (Format 13).
	StoryJSON []byte
	// Sessions are the paths of the package's sessions, sorted, such as
	// "sessions/a.troupe-session.jsonl". ReadSessions reads them.
	Sessions []string
	// Files are the paths of the package's files other than story.json and
	// its sessions, sorted. They include files nothing references, which a
	// writer keeps, but for those of extensions it implements (Format 12.3).
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
// story, as a story reader does, leaving its sessions to ReadSessions (Format
// 12.3, 15.2). It reads every entry section 12.3 allows, so data that can't be
// read is UNREADABLE wherever it is. Its results are those of ReadStory, for
// the package as a story reader reads it: the package is nil when the archive
// is UNREADABLE, or its story.json UNREADABLE or UNSUPPORTED_VERSION, and its
// diagnostics are then only those. Package.Open and Package.ReadSessions read
// r again.
//
// Before it judges the limit on entries (Format 14), it parses the archive's
// central directory, which takes memory proportional to size, so a caller
// accepting archives from others caps size.
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

// ReadSessions reads the package's sessions, by path, as a session reader does
// (Format 15.2). Its diagnostics are the package's at that level,
// ReadPackage's and the sessions', so they replace ReadPackage's. When any
// session is UNREADABLE or UNSUPPORTED_VERSION, the package fails whole: the
// sessions are nil, and the diagnostics only those (Format 15.3). The error is
// r's, when reading it fails.
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

// pkg reads the package in src, stopping when src fails. Once the archive
// opens within the ZIP limits, it reads every entry and judges story.json
// whatever else fails, so what it reports doesn't depend on the entries'
// order (Format 15.3).
func (rd *reader) pkg(src *source, size int64) *Package {
	zr, err := zip.NewReader(src, size)
	// Format 12.3's rules on names are checked below, whatever GODEBUG says.
	if err != nil && !errors.Is(err, zip.ErrInsecurePath) {
		rd.report(Diagnostic{Code: CodeUnreadable})
		return nil
	}
	// After the parse, since archive/zip checks the count an archive states
	// only modulo 65,536.
	if len(zr.File) > maxZipEntries {
		rd.report(Diagnostic{Code: CodeUnreadable})
		return nil
	}
	// archive/zip fails an entry whose data doesn't match the size its
	// header gives, so the sizes bound what decompressing produces.
	var unpacked uint64
	for _, f := range zr.File {
		unpacked += min(f.UncompressedSize64, maxUnpackedSize+1)
	}
	if unpacked > maxUnpackedSize {
		rd.report(Diagnostic{Code: CodeUnreadable})
		return nil
	}
	names := make([]string, len(zr.File))
	for i, f := range zr.File {
		names[i] = f.Name
	}
	clash := clashes(names)
	// An entry that breaks section 12.3, both entries of a clash included,
	// is no document, so it is never judged by version (Format 15.3).
	files, broken := map[string]*zip.File{}, map[*zip.File]bool{}
	for _, f := range zr.File {
		if broken[f] = !validEntry(f) || clash[asciiLower(f.Name)]; broken[f] {
			rd.reportEntry(f.Name)
		}
		if isPackageFile(f.Name) {
			files[f.Name] = f
		}
	}
	if files["story.json"] == nil {
		rd.report(Diagnostic{Code: CodeUnreadable})
	}
	rd.files = map[string]bool{}
	for name := range files {
		rd.files[name] = true
	}

	p := &Package{files: files}
	for _, f := range zr.File {
		if broken[f] {
			continue
		}
		switch {
		case files[f.Name] == nil || f.Name == "story.json":
		case isSession(f.Name):
			p.Sessions = append(p.Sessions, f.Name)
		default:
			p.Files = append(p.Files, f.Name)
		}
		if err := rd.entry(f, p); err != nil {
			if src.err != nil {
				return nil
			}
			rd.reportEntry(f.Name)
		}
	}
	if rd.unread() {
		return nil
	}
	slices.Sort(p.Sessions)
	slices.Sort(p.Files)
	return p
}

// entry reads one ZIP entry to its end, as the package's story when it is
// story.json. Its error is for data that can't be read.
func (rd *reader) entry(f *zip.File, p *Package) (err error) {
	r, err := f.Open()
	if err != nil {
		return err
	}
	defer func() { err = cmp.Or(err, r.Close()) }()
	if f.Name == "story.json" {
		rd.file = f.Name
		if p.Story, p.StoryJSON, err = rd.story(r); err != nil {
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
	drive := len(name) >= 2 && name[1] == ':' && ('A' <= name[0] && name[0] <= 'Z' || 'a' <= name[0] && name[0] <= 'z')
	absolute := strings.HasPrefix(name, "/") || drive
	return !absolute &&
		!strings.Contains(name, `\`) &&
		!strings.Contains("/"+name+"/", "/../") &&
		f.Mode()&fs.ModeSymlink == 0 &&
		f.Flags&0x1 == 0 && // encrypted
		(f.Method == zip.Store || f.Method == zip.Deflate)
}

// clashes returns, in ASCII lowercase, the names that clash: compared ASCII
// case-insensitively, equal to another, or followed by / starting another, or
// started by another so (Format 12.3). It sorts the names and scans each
// folder once, since looking up each name's folders would take time
// quadratic in a name of many segments.
func clashes(names []string) map[string]bool {
	folded := make([]string, len(names))
	for i, name := range names {
		folded[i] = asciiLower(name)
	}
	slices.Sort(folded)
	clash := map[string]bool{}
	inside := make([]bool, len(folded)) // whether a name starts with another followed by /
	for i, name := range folded {
		if i+1 < len(folded) && folded[i+1] == name {
			clash[name] = true
		}
		// Scanned already: by the name before, or, for a name inside
		// another's folder, with that folder, which holds this one's.
		if inside[i] || i > 0 && folded[i-1] == name {
			continue
		}
		folder := name + "/"
		start, _ := slices.BinarySearch(folded, folder)
		j := start
		for ; j < len(folded) && strings.HasPrefix(folded[j], folder); j++ {
			clash[folded[j]], inside[j] = true, true
		}
		if j > start {
			clash[name] = true
		}
	}
	return clash
}

// reportEntry reports UNREADABLE for a ZIP entry, located by its name only when
// that is a package path, since other names may hold anything.
func (rd *reader) reportEntry(name string) {
	d := Diagnostic{Code: CodeUnreadable}
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

// PackageWriter writes a package, a ZIP archive whose layout, names and
// limits follow Format 12.3 and 14. It writes each file as given, so the
// documents it holds conform only when the caller's do, and the caller
// creates story.json, its sessions and every file a path in them names
// (Format 12.3). A caller that rewrites a package also creates the files
// nothing names, which a writer keeps, but for those of extensions the caller
// implements.
type PackageWriter struct {
	zw       *zip.Writer
	paths    []string
	story    bool // whether story.json is created
	unpacked int64
	err      error // the first failure, after which nothing written is a package
}

// NewPackageWriter returns a PackageWriter that writes to w.
func NewPackageWriter(w io.Writer) *PackageWriter {
	return &PackageWriter{zw: zip.NewWriter(w)}
}

// Create adds the package's file at path, such as "story.json",
// "sessions/a.troupe-session.jsonl" or "images/a.webp", and returns a writer
// for its content until the next Create or Close. Its error is for a path that
// isn't a package path, for a file past the package's 10,000, or the
// writer's. Close judges how the paths clash.
func (pw *PackageWriter) Create(path string) (io.Writer, error) {
	switch {
	case pw.err != nil:
		return nil, pw.err
	case !isPackageFile(path):
		return nil, fmt.Errorf("troupe: %q isn't a package path", path)
	case len(pw.paths) == maxZipEntries:
		return nil, errors.New("troupe: the package is over its limit of files")
	}
	w, err := pw.zw.Create(path)
	if err != nil {
		pw.err = err
		return nil, err
	}
	pw.paths = append(pw.paths, path)
	pw.story = pw.story || path == "story.json"
	return &packageFile{pw, w}, nil
}

// Close finishes the package, after which Create fails. Its error is for a
// package without story.json, for two paths that, compared ASCII
// case-insensitively, are equal, or where one followed by / starts the other
// (Format 12.3), for a package past its unpacked size, or the writer's.
func (pw *PackageWriter) Close() error {
	switch {
	case pw.err != nil:
		return pw.err
	case !pw.story:
		return errors.New("troupe: the package has no story.json")
	}
	clash := clashes(pw.paths)
	for _, path := range slices.Sorted(slices.Values(pw.paths)) {
		if clash[asciiLower(path)] {
			return fmt.Errorf("troupe: %q clashes with another file's path", path)
		}
	}
	pw.err = errors.New("troupe: the package is closed")
	return pw.zw.Close()
}

// packageFile writes a file's content, counting it toward the package's
// unpacked size, which it fails past (Format 14).
type packageFile struct {
	pw *PackageWriter
	w  io.Writer
}

func (f *packageFile) Write(p []byte) (int, error) {
	if f.pw.err != nil {
		return 0, f.pw.err
	}
	if f.pw.unpacked += int64(len(p)); f.pw.unpacked > maxUnpackedSize {
		f.pw.err = errors.New("troupe: the package is over its unpacked size")
		return 0, f.pw.err
	}
	n, err := f.w.Write(p)
	if err != nil {
		f.pw.err = err
	}
	return n, err
}
