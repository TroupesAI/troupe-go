package troupe

import (
	"bufio"
	"bytes"
	"encoding/json/jsontext"
	"errors"
	"io"
	"regexp"
	"slices"
	"strconv"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/TroupesAI/troupe-go/internal/ijson"
)

// ReadStory reads a story file (Format 3, 12.1). It returns the diagnostics of
// UNREADABLE, UNSUPPORTED_VERSION, SCHEMA and UNKNOWN_FIELD, sorted by
// location. The story is nil when it is UNREADABLE or UNSUPPORTED_VERSION;
// with other diagnostics it serves only to report them. The error is r's, when
// reading it fails.
func ReadStory(r io.Reader) (*Story, []Diagnostic, error) {
	var rd reader
	st, _, err := rd.story(r)
	if err != nil {
		return nil, nil, err
	}
	return st, rd.diagnostics(), nil
}

// ReadPackageStory reads story.json as a package holds it, without the
// package. Files are the paths of the package's files, such as Package.Files
// and Package.Sessions, and its package paths must name one of them or
// story.json (Format 12.2, 12.3). Its results are those ReadPackage gives for
// that story.json, so every diagnostic's File is "story.json".
func ReadPackageStory(r io.Reader, files []string) (*Story, []Diagnostic, error) {
	rd := reader{file: "story.json", files: map[string]bool{"story.json": true}}
	for _, f := range files {
		rd.files[f] = true
	}
	st, _, err := rd.story(r)
	if err != nil {
		return nil, nil, err
	}
	return st, rd.diagnostics(), nil
}

// ReadSession reads a session file, line by line (Format 11). Its results are
// those of ReadStory, for the whole file: the session is nil when any line is
// UNREADABLE, or its header UNSUPPORTED_VERSION, and its diagnostics are then
// only those.
func ReadSession(r io.Reader) (*Session, []Diagnostic, error) {
	var rd reader
	s, err := rd.session(r)
	if err != nil {
		return nil, nil, err
	}
	return s, rd.diagnostics(), nil
}

// reader reads the documents of one input and collects their diagnostics.
type reader struct {
	file  string          // the path within a package
	files map[string]bool // the paths of a package's files; nil outside a package
	diags []Diagnostic
}

var errTooLarge = errors.New("troupe: over the size limit")

// readAll reads r to its end, failing with errTooLarge past limit bytes.
func readAll(r io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err == nil && int64(len(data)) > limit {
		err = errTooLarge
	}
	return data, err
}

// story reads a story, and returns it with its bytes.
func (rd *reader) story(r io.Reader) (*Story, []byte, error) {
	data, err := readAll(r, maxStorySize)
	if errors.Is(err, errTooLarge) {
		rd.report(Diagnostic{Code: CodeUnreadable})
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	tree, ok := rd.parse(data, 0)
	if !ok {
		return nil, nil, nil
	}
	if otherVersion(tree) {
		rd.report(Diagnostic{Code: CodeUnsupportedVersion, Pointer: "/troupe"})
		return nil, nil, nil
	}
	var st Story
	if !rd.document(tree, data, 0, storyDoc, &st) {
		return nil, nil, nil
	}
	return &st, data, nil
}

func (rd *reader) session(r io.Reader) (*Session, error) {
	var s Session
	ok, other := true, false
	br := bufio.NewReader(io.LimitReader(r, maxSessionSize+1))
	size, line := 0, 0
	for {
		data, err := br.ReadBytes('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
		if len(data) == 0 {
			break
		}
		line++
		if size += len(data); size > maxSessionSize {
			rd.report(Diagnostic{Code: CodeUnreadable, Line: line})
			return nil, nil
		}
		// Past its header, a session of another version is judged only by
		// its size (Format 15.3).
		if other {
			continue
		}
		// Lines end with \n or \r\n, and none is empty (Format 11).
		data = bytes.TrimSuffix(bytes.TrimSuffix(data, []byte("\n")), []byte("\r"))
		switch {
		case line == 1:
			// Without its header, a session's other lines mean nothing.
			tree, read := rd.parse(data, line)
			if !read {
				return nil, nil
			}
			if other = otherVersion(tree); !other && !rd.document(tree, data, line, headerDoc, &s.Header) {
				return nil, nil
			}
		case len(data) == 0:
			rd.report(Diagnostic{Code: CodeUnreadable, Line: line})
			ok = false
		case line-1 > maxMessages:
			rd.report(Diagnostic{Code: CodeUnreadable, Line: line})
			return nil, nil
		default:
			tree, read := rd.parse(data, line)
			var m Message
			ok = read && rd.document(tree, data, line, messageDoc, &m) && ok
			s.Messages = append(s.Messages, m)
		}
	}
	if line == 0 {
		rd.report(Diagnostic{Code: CodeUnreadable, Line: 1})
		return nil, nil
	}
	if other {
		rd.report(Diagnostic{Code: CodeUnsupportedVersion, Line: 1, Pointer: "/troupe"})
		return nil, nil
	}
	if !ok || !rd.sessionEntries(&s) {
		return nil, nil
	}
	return &s, nil
}

// sessionEntries checks that no path holds more than maxSessionEntries
// session entries, counting the latest per ID (Format 11.3, 14).
func (rd *reader) sessionEntries(s *Session) bool {
	onPath := map[string]int{}
	ok := true
	walkPaths(s.Messages, func(i int) {
		for j, e := range s.Messages[i].Entries {
			if onPath[e.ID]++; onPath[e.ID] == 1 && len(onPath) == maxSessionEntries+1 {
				at := inMessage(i, &s.Messages[i])
				rd.report(Diagnostic{Code: CodeUnreadable, Line: at.line, Message: at.message, Pointer: "/entries/" + strconv.Itoa(j)})
				ok = false
			}
		}
	}, func(i int) {
		for _, e := range s.Messages[i].Entries {
			if onPath[e.ID]--; onPath[e.ID] == 0 {
				delete(onPath, e.ID)
			}
		}
	})
	return ok
}

// docKind is a kind of document: a story, a session header or a message.
type docKind struct {
	kind   string // the document's kind field; "" for a message, which has none
	schema func() *jsonschema.Schema
	scan   func(*scan, map[string]any)
}

var (
	storyDoc   = docKind{"story", func() *jsonschema.Schema { return schemas().story }, (*scan).story}
	headerDoc  = docKind{"session", func() *jsonschema.Schema { return schemas().header }, (*scan).header}
	messageDoc = docKind{"", func() *jsonschema.Schema { return schemas().message }, (*scan).message}
)

var (
	version   = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
	messageID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
)

// parse reads one document, at its line of a session or 0 for a story, as
// I-JSON within the nesting depth, and reports UNREADABLE when it isn't
// (Format 15.3).
func (rd *reader) parse(data []byte, line int) (any, bool) {
	tree, err := ijson.Decode(data)
	if e, ok := errors.AsType[*ijson.Error](err); ok {
		rd.report(Diagnostic{Code: CodeUnreadable, Line: line, Pointer: string(e.Pointer)})
		return nil, false
	}
	return tree, true
}

// otherVersion reports whether a story or a session header names a version
// other than this one; nothing else in it is then judged (Format 13, 15.3). A
// missing troupe, or one that isn't a version, is the schema's to report.
func otherVersion(tree any) bool {
	doc, _ := tree.(map[string]any)
	t, ok := doc["troupe"].(string)
	return ok && version.MatchString(t) && t != Version
}

// document judges a parsed document of this version, at its line of a
// session or 0 for a story, and decodes it into v. It reports false when the
// document is UNREADABLE.
func (rd *reader) document(tree any, data []byte, line int, k docKind, v any) bool {
	at := Diagnostic{Line: line}
	doc, _ := tree.(map[string]any)
	if id, ok := doc["id"].(string); ok && line > 1 {
		at.Message = validID(id)
	}
	report := func(c Code, p jsontext.Pointer) {
		d := at
		d.Code, d.Pointer = c, string(p)
		rd.report(d)
	}
	// A document with a missing or other kind isn't one its form holds, and
	// the limits are those of its kind (Format 15.3). A message has no kind,
	// so one that isn't an object is only SCHEMA.
	if k.kind != "" && doc["kind"] != k.kind {
		report(CodeUnreadable, "")
		return false
	}
	sc := scan{report: func(p jsontext.Pointer) { report(CodeUnreadable, p) }, files: rd.files}
	k.scan(&sc, doc)
	if sc.failed {
		return false
	}
	validate(k.schema(), tree, report)
	decode(data, v)
	return true
}

func (rd *reader) report(d Diagnostic) {
	d.File = rd.file
	rd.diags = append(rd.diags, d)
}

// diagnostics returns the input's diagnostics, sorted. An input, a whole file
// or package, that is UNREADABLE or UNSUPPORTED_VERSION is reported with those
// codes only (Format 15.3).
func (rd *reader) diagnostics() []Diagnostic {
	if rd.unread() {
		rd.diags = slices.DeleteFunc(rd.diags, func(d Diagnostic) bool { return !unread(d) })
	}
	return sortDiagnostics(rd.diags)
}

// unread reports whether the input is UNREADABLE or UNSUPPORTED_VERSION.
func (rd *reader) unread() bool {
	return slices.ContainsFunc(rd.diags, unread)
}

func unread(d Diagnostic) bool {
	return d.Code == CodeUnreadable || d.Code == CodeUnsupportedVersion
}
