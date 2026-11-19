package troupe

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"iter"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// FileField is a field that holds a file, and its value: an https: URL, a
// package path or a data: URI, whichever sets its fields (Format 12.2). A
// data: URI's content isn't checked against its media type: readers leave
// that to the app that shows it.
type FileField struct {
	Line      int    // the session line holding the field, from 1 for its header; 0 in a story
	Pointer   string // the field, as a JSON Pointer into its document
	URL       string // an https: URL
	Path      string // a package path
	MediaType string // a data: URI's media type, such as "image/png"
	Data      []byte // a data: URI's content, decoded
}

// FileFields yields the story's fields that hold a file: its cover, then its
// characters' avatars, its locations' images and the image parts of its
// openings. It decodes each data: URI as it yields it, so only one is in
// memory at a time. In a story that fails the schema, a value of none of the
// three forms is left out.
func (st *Story) FileFields() iter.Seq[FileField] {
	return func(yield func(FileField) bool) {
		fileFields(0, storySlots(st), yield)
	}
}

// FileFields yields the session's fields that hold a file, as
// Story.FileFields does, each with its line: its header's characters'
// avatars and its locations' images, then its messages' image parts.
func (s *Session) FileFields() iter.Seq[FileField] {
	return func(yield func(FileField) bool) {
		if !fileFields(1, headerSlots(&s.Header), yield) {
			return
		}
		for i := range s.Messages {
			if !fileFields(i+2, contentSlots("", s.Messages[i].Content), yield) {
				return
			}
		}
	}
}

// fileFields yields the fields of a document's slots that hold a file,
// reporting false when yield stops.
func fileFields(line int, slots iter.Seq[slot], yield func(FileField) bool) bool {
	for s := range slots {
		if f, ok := fileValue(s.value); ok {
			f.Line, f.Pointer = line, s.pointer
			if !yield(f) {
				return false
			}
		}
	}
	return true
}

// slot is a field that can hold a file, whatever its value. For an image
// part, content is the pointer of the content holding it, and parts the
// number of parts there.
type slot struct {
	pointer, value string
	content        string
	parts          int
}

func storySlots(st *Story) iter.Seq[slot] {
	return func(yield func(slot) bool) {
		if !yield(slot{pointer: "/cover", value: st.Cover}) || !ownerSlots(st.Characters, st.Worlds, yield) {
			return
		}
		for i, s := range st.Scenarios {
			for j, o := range s.Openings {
				for f := range contentSlots(fmt.Sprintf("/scenarios/%d/openings/%d", i, j), o.Content) {
					if !yield(f) {
						return
					}
				}
			}
		}
	}
}

func headerSlots(h *SessionHeader) iter.Seq[slot] {
	return func(yield func(slot) bool) {
		ownerSlots(h.Characters, h.Worlds, yield)
	}
}

// ownerSlots yields the characters' avatars and the locations' images,
// reporting false when yield stops.
func ownerSlots(characters []Character, worlds []World, yield func(slot) bool) bool {
	for i, c := range characters {
		if !yield(slot{pointer: fmt.Sprintf("/characters/%d/avatar", i), value: c.Avatar}) {
			return false
		}
	}
	for i, w := range worlds {
		for j, l := range w.Locations {
			if !yield(slot{pointer: fmt.Sprintf("/worlds/%d/locations/%d/image", i, j), value: l.Image}) {
				return false
			}
		}
	}
	return true
}

// contentSlots yields the image parts of the content of the object at p.
func contentSlots(p string, content []Part) iter.Seq[slot] {
	return func(yield func(slot) bool) {
		c := p + "/content"
		for k, part := range content {
			if !yield(slot{pointer: c + "/" + strconv.Itoa(k) + "/src", value: part.Src, content: c, parts: len(content)}) {
				return
			}
		}
	}
}

var dataURI = regexp.MustCompile(`^data:(image/(?:png|jpeg|webp|avif|gif));base64,`)

// fileValue reads the value of a field that holds a file, reporting false
// when it has none of the forms of Format 12.2, as the schema judges them.
func fileValue(v string) (FileField, bool) {
	if schemas().image.Validate(v) != nil {
		return FileField{}, false
	}
	switch {
	case packagePath.MatchString(v):
		return FileField{Path: v}, true
	case strings.HasPrefix(v, "https://"):
		return FileField{URL: v}, true
	}
	m := dataURI.FindStringSubmatch(v)
	data, _ := base64.StdEncoding.DecodeString(v[len(m[0]):]) // the schema's pattern is base64
	return FileField{MediaType: m[1], Data: data}, true
}

// ReplaceFileFields returns doc, a story's JSON, with the fields that hold a
// file at the pointers in values, as Story.FileFields gives them, set to their
// values. A value of "" removes its field: an image part goes whole, with its
// ext, since it can't be without its src, and an opening that loses every
// part loses its content, so a model writes its first message (Format 6.3).
// Every other byte is kept, so the ext of everything kept stays as written
// (Format 13), but for the comma or whitespace that separated what is
// removed. Its error is for doc that isn't JSON, a pointer that names no field
// FileFields gives, or a value that isn't "" or an https: URL, a data: URI or
// a package path as Format 12.2 allows them.
func ReplaceFileFields(doc []byte, values map[string]string) ([]byte, error) {
	var st Story
	if json.Unmarshal(doc, &st, lenient) != nil {
		return nil, errors.New("troupe: the document isn't one JSON value")
	}
	out, err := replaceFileFields(doc, storySlots(&st), values, func() (bool, error) { return true, nil })
	if err != nil {
		return nil, fmt.Errorf("troupe: %w", err)
	}
	return out, nil
}

// ReplaceSessionFileFields writes the session read from r to w, with its
// fields that hold a file set as ReplaceFileFields sets a story's: values
// gives, by line, the values for that line's fields at their pointers, as
// Session.FileFields gives them. A message that loses every part keeps an
// empty content, which only a system message may have (Format 11): a root
// whose opening loses every part as well, since a root holds its opening's
// content (Format 11.1, 11.4). Every other line, and every line's ending, is
// written as read. It holds one line
// in memory at a time. Its error is r's or w's, or for a session past its
// size limit (Format 14), a line the session hasn't, or one ReplaceFileFields
// would give for that line, such as for a player's or a model's message left
// without content. After an error, what w holds is no session.
func ReplaceSessionFileFields(w io.Writer, r io.Reader, values map[int]map[string]string) error {
	br := bufio.NewReader(io.LimitReader(r, maxSessionSize+1))
	size, line := 0, 0
	for {
		data, err := br.ReadBytes('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		if len(data) == 0 {
			break
		}
		line++
		if size += len(data); size > maxSessionSize {
			return errors.New("troupe: the session is over its size limit")
		}
		if v, ok := values[line]; ok {
			if data, err = replaceLine(line, data, v); err != nil {
				return fmt.Errorf("troupe: line %d: %w", line, err)
			}
		}
		if _, err := w.Write(data); err != nil {
			return err
		}
	}
	for _, l := range slices.Sorted(maps.Keys(values)) {
		if l < 1 || l > line {
			return fmt.Errorf("troupe: the session has no line %d", l)
		}
	}
	return nil
}

// replaceLine replaces the file fields of a session's line, data, keeping
// its ending (Format 11).
func replaceLine(line int, data []byte, values map[string]string) ([]byte, error) {
	doc := bytes.TrimSuffix(bytes.TrimSuffix(data, []byte("\n")), []byte("\r"))
	end := data[len(doc):]
	var out []byte
	var err error
	if line == 1 {
		var h SessionHeader
		if json.Unmarshal(doc, &h, lenient) != nil {
			return nil, errNotLine
		}
		out, err = replaceFileFields(doc, headerSlots(&h), values, nil) // a header has no content
	} else {
		var m Message
		if json.Unmarshal(doc, &m, lenient) != nil {
			return nil, errNotLine
		}
		out, err = replaceFileFields(doc, contentSlots("", m.Content), values, func() (bool, error) {
			if m.From != "system" {
				return false, errors.New("a player's or a model's message can't be left without content")
			}
			return false, nil
		})
	}
	return append(out, end...), err
}

var errNotLine = errors.New("the line isn't one JSON value")

// replaceFileFields returns doc, the document slots come from, with values
// set as ReplaceFileFields sets them. Of a content that loses every part,
// emptied says whether it goes, or why it can't be left empty.
func replaceFileFields(doc []byte, slots iter.Seq[slot], values map[string]string, emptied func() (bool, error)) ([]byte, error) {
	fields := map[string]slot{}
	for s := range slots {
		if _, named := values[s.pointer]; named && schemas().image.Validate(s.value) == nil {
			fields[s.pointer] = s
		}
	}
	rw := rewrite{doc: doc, within: map[jsontext.Pointer]bool{}, replace: map[jsontext.Pointer][]byte{}, remove: map[jsontext.Pointer]bool{}}
	lost := map[string]int{} // parts removed, by content
	for _, p := range slices.Sorted(maps.Keys(values)) {
		f, ok := fields[p]
		if !ok {
			return nil, fmt.Errorf("%q names no field that holds a file", p)
		}
		v := values[p]
		if v != "" {
			q, err := jsontext.AppendQuote(nil, v)
			if err != nil || schemas().image.Validate(v) != nil {
				return nil, fmt.Errorf("the value for %q holds no file", p)
			}
			rw.replace[jsontext.Pointer(p)] = q
			continue
		}
		if f.content == "" {
			rw.remove[jsontext.Pointer(p)] = true
			continue
		}
		rw.remove[jsontext.Pointer(p).Parent()] = true
		if lost[f.content]++; lost[f.content] == f.parts {
			gone, err := emptied()
			if err != nil {
				return nil, err
			}
			if gone {
				rw.remove[jsontext.Pointer(f.content)] = true
			}
		}
	}
	for p := range rw.replace {
		rw.mark(p)
	}
	for p := range rw.remove {
		rw.mark(p.Parent())
	}
	rw.dec = jsontext.NewDecoder(bytes.NewReader(doc))
	// doc decoded, so reading it can't fail.
	rw.value("")
	out := make([]byte, 0, len(doc))
	last := int64(0)
	for _, s := range rw.spans {
		out = append(append(out, doc[last:s.start]...), s.value...)
		last = s.end
	}
	return append(out, doc[last:]...), nil
}

// rewrite finds the spans of a document to replace and remove.
type rewrite struct {
	doc     []byte
	dec     *jsontext.Decoder
	within  map[jsontext.Pointer]bool // the values holding one to change, and those changed
	replace map[jsontext.Pointer][]byte
	remove  map[jsontext.Pointer]bool
	spans   []span // in document order
}

// span is the bytes of a document from start to end, which value replaces.
type span struct {
	start, end int64
	value      []byte
}

// mark marks p and the values holding it as within.
func (rw *rewrite) mark(p jsontext.Pointer) {
	for ; !rw.within[p]; p = p.Parent() {
		rw.within[p] = true
		if p == "" {
			return
		}
	}
}

// value reads the value at p, descending only into what holds one to change.
func (rw *rewrite) value(p jsontext.Pointer) {
	if !rw.within[p] {
		_ = rw.dec.SkipValue()
		return
	}
	kind := rw.dec.PeekKind()
	if kind == '"' {
		v, _ := rw.dec.ReadValue()
		if q, ok := rw.replace[p]; ok {
			end := rw.dec.InputOffset()
			rw.spans = append(rw.spans, span{end - int64(len(v)), end, q})
		}
		return
	}
	if kind != '{' && kind != '[' {
		_ = rw.dec.SkipValue()
		return
	}
	_, _ = rw.dec.ReadToken()
	kept := false // whether a member or item before this one is kept
	for i := 0; rw.dec.PeekKind() != '}' && rw.dec.PeekKind() != ']'; i++ {
		before := rw.dec.InputOffset()
		child := p.AppendToken(strconv.Itoa(i))
		if kind == '{' {
			name, _ := rw.dec.ReadToken()
			child = p.AppendToken(name.String())
		}
		if !rw.remove[child] {
			kept = true
			rw.value(child)
			continue
		}
		_ = rw.dec.SkipValue()
		// Removed with the comma before it, or, when nothing before it is
		// kept, the comma after it, so what is left stays JSON.
		start, end := separated(rw.doc, before), rw.dec.InputOffset()
		if kept {
			start = before
		} else if rw.doc[skipSpace(rw.doc, end)] == ',' {
			end = separated(rw.doc, end)
		}
		rw.spans = append(rw.spans, span{start, end, nil})
	}
	_, _ = rw.dec.ReadToken()
}

// separated returns the offset of the first byte from off that is neither
// JSON whitespace nor the comma that may separate two values.
func separated(doc []byte, off int64) int64 {
	off = skipSpace(doc, off)
	if doc[off] == ',' {
		off = skipSpace(doc, off+1)
	}
	return off
}

// skipSpace returns the offset of the first byte from off that isn't JSON
// whitespace.
func skipSpace(doc []byte, off int64) int64 {
	for strings.IndexByte(" \t\r\n", doc[off]) >= 0 {
		off++
	}
	return off
}
