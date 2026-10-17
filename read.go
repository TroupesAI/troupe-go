package troupe

import (
	"bufio"
	"bytes"
	"encoding/json/jsontext"
	"errors"
	"io"
	"regexp"
	"strconv"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/TroupesAI/troupe-go/internal/ijson"
)

// ReadBundle reads a bundle's JSON, as a single file holds it (Format 3, 12.1).
// It returns the diagnostics of PARSE, UNSUPPORTED_VERSION, SCHEMA and
// UNKNOWN_FIELD, sorted by location. The bundle is nil when it fails PARSE or
// UNSUPPORTED_VERSION; with other diagnostics it serves only to report them.
// The error is r's, when reading it fails.
func ReadBundle(r io.Reader) (*Bundle, []Diagnostic, error) {
	var rd reader
	b, err := rd.bundle(r)
	if err != nil {
		return nil, nil, err
	}
	return b, sortDiagnostics(rd.diags), nil
}

// ReadSession reads a session file, line by line (Format 11). Its results are
// those of ReadBundle, for every line: the session is nil when any line fails
// PARSE, or its header UNSUPPORTED_VERSION.
func ReadSession(r io.Reader) (*Session, []Diagnostic, error) {
	var rd reader
	s, err := rd.session(r)
	if err != nil {
		return nil, nil, err
	}
	return s, sortDiagnostics(rd.diags), nil
}

// reader reads the documents of one input and collects their diagnostics.
type reader struct {
	file  string            // the path within a package
	files func(string) bool // a package's files; nil outside a package
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

func (rd *reader) bundle(r io.Reader) (*Bundle, error) {
	data, err := readAll(r, maxBundleSize)
	if errors.Is(err, errTooLarge) {
		rd.report(Diagnostic{Code: CodeParse})
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var b Bundle
	if !rd.document(data, 0, bundleDoc, &b) {
		return nil, nil
	}
	return &b, nil
}

func (rd *reader) session(r io.Reader) (*Session, error) {
	var s Session
	ok := true
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
			rd.report(Diagnostic{Code: CodeParse, Line: line})
			return nil, nil
		}
		// Lines end with \n or \r\n, and none is empty (Format 11).
		data = bytes.TrimSuffix(bytes.TrimSuffix(data, []byte("\n")), []byte("\r"))
		switch {
		case line == 1:
			// Without its header, a session's other lines mean nothing.
			if len(data) == 0 {
				rd.report(Diagnostic{Code: CodeParse, Line: line})
				return nil, nil
			}
			if !rd.document(data, line, headerDoc, &s.Header) {
				return nil, nil
			}
		case len(data) == 0:
			rd.report(Diagnostic{Code: CodeParse, Line: line})
			ok = false
		case line-1 > maxMessages:
			rd.report(Diagnostic{Code: CodeParse, Line: line})
			return nil, nil
		default:
			var m Message
			ok = rd.document(data, line, messageDoc, &m) && ok
			s.Messages = append(s.Messages, m)
		}
	}
	if line == 0 {
		rd.report(Diagnostic{Code: CodeParse, Line: 1})
		return nil, nil
	}
	if !ok || !rd.sessionEntries(&s) {
		return nil, nil
	}
	return &s, nil
}

// sessionEntries checks that no path holds more than maxSessionEntries
// session entries, counting the latest per ID (Format 11.3, 14). It walks the
// message tree depth first, counting the IDs on the path to each message.
func (rd *reader) sessionEntries(s *Session) bool {
	roots := len(s.Messages) // the index whose children are the roots
	children := make([][]int, roots+1)
	index := map[string]int{}
	for i, m := range s.Messages {
		parent, ok := index[m.Parent]
		if !ok {
			parent = roots
		}
		children[parent] = append(children[parent], i)
		if _, dup := index[m.ID]; !dup {
			index[m.ID] = i
		}
	}
	type visit struct {
		message int
		leave   bool
	}
	var stack []visit
	for _, c := range children[roots] {
		stack = append(stack, visit{c, false})
	}
	onPath := map[string]int{}
	ok := true
	for len(stack) > 0 {
		v := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		m := &s.Messages[v.message]
		if v.leave {
			for _, e := range m.Entries {
				if onPath[e.ID]--; onPath[e.ID] == 0 {
					delete(onPath, e.ID)
				}
			}
			continue
		}
		for j, e := range m.Entries {
			if onPath[e.ID]++; onPath[e.ID] == 1 && len(onPath) == maxSessionEntries+1 {
				rd.report(Diagnostic{
					Code:    CodeParse,
					Line:    v.message + 2,
					Message: m.ID,
					Pointer: "/entries/" + strconv.Itoa(j),
				})
				ok = false
			}
		}
		stack = append(stack, visit{v.message, true})
		for _, c := range children[v.message] {
			stack = append(stack, visit{c, false})
		}
	}
	return ok
}

// docKind is a kind of document: a bundle, a session header or a message.
type docKind struct {
	kind   string // the document's kind field; "" for a message, which has none
	schema func() *jsonschema.Schema
	scan   func(*scan, map[string]any)
}

var (
	bundleDoc  = docKind{"bundle", func() *jsonschema.Schema { return schemas().bundle }, (*scan).bundle}
	headerDoc  = docKind{"session", func() *jsonschema.Schema { return schemas().header }, (*scan).header}
	messageDoc = docKind{"", func() *jsonschema.Schema { return schemas().message }, (*scan).message}
)

var (
	version   = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
	messageID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
)

// document reads one document, at its line of a session or 0 for a bundle,
// and decodes it into v. It reports false when the document fails PARSE or
// UNSUPPORTED_VERSION, after which no other check runs: the others would judge
// what can't be read, or another version, by this version's rules.
func (rd *reader) document(data []byte, line int, k docKind, v any) bool {
	at := Diagnostic{Line: line}
	tree, err := ijson.Decode(data)
	if e, ok := errors.AsType[*ijson.Error](err); ok {
		at.Code, at.Pointer = CodeParse, string(e.Pointer)
		rd.report(at)
		return false
	}
	doc, _ := tree.(map[string]any)
	if id, ok := doc["id"].(string); ok && line > 1 && messageID.MatchString(id) {
		at.Message = id
	}
	report := func(c Code, p jsontext.Pointer) {
		d := at
		d.Code, d.Pointer = c, string(p)
		rd.report(d)
	}
	if k.kind != "" {
		// A document of another kind isn't one its form holds (Format 15.3).
		if doc["kind"] != k.kind {
			report(CodeParse, "")
			return false
		}
		if t, ok := doc["troupe"].(string); ok && version.MatchString(t) && t != Version {
			report(CodeUnsupportedVersion, "/troupe")
			return false
		}
	}
	parsed := len(rd.diags)
	sc := scan{report: func(p jsontext.Pointer) { report(CodeParse, p) }, files: rd.files}
	k.scan(&sc, doc)
	if len(rd.diags) > parsed {
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
