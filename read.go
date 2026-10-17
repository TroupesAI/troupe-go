package troupe

import (
	"archive/zip"
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
	b, _, err := rd.bundle(r)
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
	file  string               // the path within a package
	files map[string]*zip.File // a package's files; nil outside a package
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

// bundle reads a bundle, and returns it with its bytes.
func (rd *reader) bundle(r io.Reader) (*Bundle, []byte, error) {
	data, err := readAll(r, maxBundleSize)
	if errors.Is(err, errTooLarge) {
		rd.report(Diagnostic{Code: CodeParse})
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var b Bundle
	if !rd.document(data, 0, bundleDoc, &b) {
		return nil, nil, nil
	}
	return &b, data, nil
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
// session entries, counting the latest per ID (Format 11.3, 14).
func (rd *reader) sessionEntries(s *Session) bool {
	onPath := map[string]int{}
	ok := true
	walkPaths(s.Messages, func(i int) {
		for j, e := range s.Messages[i].Entries {
			if onPath[e.ID]++; onPath[e.ID] == 1 && len(onPath) == maxSessionEntries+1 {
				at := inMessage(i, &s.Messages[i])
				rd.report(Diagnostic{Code: CodeParse, Line: at.line, Message: at.message, Pointer: "/entries/" + strconv.Itoa(j)})
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
// UNSUPPORTED_VERSION, which it is then reported with only (Format 15.3).
func (rd *reader) document(data []byte, line int, k docKind, v any) bool {
	at := Diagnostic{Line: line}
	tree, err := ijson.Decode(data)
	if e, ok := errors.AsType[*ijson.Error](err); ok {
		at.Code, at.Pointer = CodeParse, string(e.Pointer)
		rd.report(at)
		return false
	}
	doc, _ := tree.(map[string]any)
	if id, ok := doc["id"].(string); ok && line > 1 {
		at.Message = validID(id)
	}
	report := func(c Code, p jsontext.Pointer) {
		d := at
		d.Code, d.Pointer = c, string(p)
		rd.report(d)
	}
	if k.kind != "" {
		// A document with a missing or other kind isn't one its form holds
		// (Format 15.3).
		kind := doc["kind"] == k.kind
		if !kind {
			report(CodeParse, "")
		}
		t, ok := doc["troupe"].(string)
		supported := !ok || !version.MatchString(t) || t == Version
		if !supported {
			report(CodeUnsupportedVersion, "/troupe")
		}
		// The limits are this version's, for this kind of document.
		if !kind || !supported {
			return false
		}
	}
	sc := scan{report: func(p jsontext.Pointer) { report(CodeParse, p) }, files: rd.files}
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
