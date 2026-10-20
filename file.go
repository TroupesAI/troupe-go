package troupe

import (
	"bytes"
	"encoding/base64"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"iter"
	"maps"
	"regexp"
	"slices"
	"strings"
)

// FileField is a field that holds a file, and its value: an https: URL, a
// package path or a data: URI, whichever sets its fields (Format 12.2). A
// data: URI's content isn't checked against its media type: readers leave
// that to the app that shows it.
type FileField struct {
	Pointer   string // the field, as a JSON Pointer into its document
	URL       string // an https: URL
	Path      string // a package path
	MediaType string // a data: URI's media type, such as "image/png"
	Data      []byte // a data: URI's content, decoded
}

// FileFields yields the bundle's fields that hold a file: its cover, then its
// characters' avatars, its locations' images and the image parts of its
// openings. It decodes each data: URI as it yields it, so only one is in
// memory at a time. In a bundle that fails the schema, a value of none of the
// three forms is left out.
func (b *Bundle) FileFields() iter.Seq[FileField] {
	return func(yield func(FileField) bool) {
		field := func(pointer, v string) bool {
			f, ok := fileValue(v)
			f.Pointer = pointer
			return !ok || yield(f)
		}
		if !field("/cover", b.Cover) {
			return
		}
		for i, c := range b.Characters {
			if !field(fmt.Sprintf("/characters/%d/avatar", i), c.Avatar) {
				return
			}
		}
		for i, w := range b.Worlds {
			for j, l := range w.Locations {
				if !field(fmt.Sprintf("/worlds/%d/locations/%d/image", i, j), l.Image) {
					return
				}
			}
		}
		for i, s := range b.Scenarios {
			for j, o := range s.Openings {
				for k, part := range o.Content {
					if !field(fmt.Sprintf("/scenarios/%d/openings/%d/content/%d/src", i, j, k), part.Src) {
						return
					}
				}
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

// ReplaceFileFields returns doc, a document's JSON, with the fields that hold
// a file at the pointers in values, as FileFields gives them, set to their
// values. Every other byte is kept, so every ext stays as written (Format 13).
// Its error is for doc that isn't JSON, a pointer that names no string in doc,
// or a value that isn't an https: URL, a data: URI or a package path as Format
// 12.2 allows them.
func ReplaceFileFields(doc []byte, values map[string]string) ([]byte, error) {
	quoted := map[string][]byte{}
	for _, p := range slices.Sorted(maps.Keys(values)) {
		q, err := jsontext.AppendQuote(nil, values[p])
		if err != nil || schemas().image.Validate(values[p]) != nil {
			return nil, fmt.Errorf("troupe: the value for %q holds no file", p)
		}
		quoted[p] = q
	}
	type span struct {
		start, end int64
		value      []byte
	}
	if !jsontext.Value(doc).IsValid() {
		return nil, errors.New("troupe: the document isn't one JSON value")
	}
	var spans []span
	dec := jsontext.NewDecoder(bytes.NewReader(doc))
	// The document is valid, so reading it can't fail, and its kind is 0
	// only at its end.
	for k := dec.PeekKind(); k != 0; k = dec.PeekKind() {
		if kind, n := dec.StackIndex(dec.StackDepth()); k == '{' || k == '[' || k == '}' || k == ']' || kind == '{' && n%2 == 0 {
			_, _ = dec.ReadToken()
			continue
		}
		v, _ := dec.ReadValue()
		p := string(dec.StackPointer())
		if q, ok := quoted[p]; ok && v.Kind() == '"' {
			end := dec.InputOffset()
			spans = append(spans, span{end - int64(len(v)), end, q})
			delete(quoted, p)
		}
	}
	if len(quoted) > 0 {
		return nil, fmt.Errorf("troupe: %q names no string", slices.Min(slices.Collect(maps.Keys(quoted))))
	}
	out := make([]byte, 0, len(doc))
	last := int64(0)
	for _, s := range spans {
		out = append(append(out, doc[last:s.start]...), s.value...)
		last = s.end
	}
	return append(out, doc[last:]...), nil
}
