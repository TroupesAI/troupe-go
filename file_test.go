package troupe_test

import (
	"io"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/TroupesAI/troupe-go"
)

// FileFields yields every field that holds a file, with its pointer and its
// form, and leaves out values of no form, which a story that fails the
// schema can hold.
func TestFileFields(t *testing.T) {
	st := &troupe.Story{
		Cover:      "https://x.test/cover.png",
		Characters: []troupe.Character{{}, {Avatar: "images/kai.png"}},
		Worlds: []troupe.World{{Locations: []troupe.Location{
			{Image: "data:image/gif;base64,R0lG"}, {Image: "data:text/plain;base64,R0lG"}, {Image: "https://x.test/a b.png"},
		}}},
		Scenarios: []troupe.Scenario{{Openings: []troupe.Opening{{Content: []troupe.Part{
			{Text: "Hi."},
			{Type: "image", Src: "map.webp"},
		}}}}},
	}
	want := []troupe.FileField{
		{Pointer: "/cover", URL: "https://x.test/cover.png"},
		{Pointer: "/characters/1/avatar", Path: "images/kai.png"},
		{Pointer: "/worlds/0/locations/0/image", MediaType: "image/gif", Data: []byte("GIF")},
		{Pointer: "/scenarios/0/openings/0/content/1/src", Path: "map.webp"},
	}
	if got := slices.Collect(st.FileFields()); !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
}

// ReplaceFileFields fails, rather than writing a document that doesn't
// conform, for a pointer that names no field that holds a file, or a value
// that holds no file.
func TestReplaceFileFieldsErrors(t *testing.T) {
	doc := []byte(`{"name":"Heist","cover":"a.png","characters":[{"avatar":"b.png"}]}`)
	for _, values := range []map[string]string{
		{"/cover": "c.png", "/avatar": "c.png"},
		{"/characters": "c.png"},
		{"/name": ""},
		{"/cover": "http://x.test/c.png"},
		{"/cover": "data:image/svg+xml;base64,R0lG"},
	} {
		if _, err := troupe.ReplaceFileFields(doc, values); err == nil {
			t.Errorf("%v: no error", values)
		}
	}
	for _, doc := range []string{`{"cover":"a.png"`, `{"cover":"a.png"} {"cover":"b.png"}`} {
		if _, err := troupe.ReplaceFileFields([]byte(doc), map[string]string{"/cover": "c.png"}); err == nil {
			t.Errorf("%s: no error", doc)
		}
	}
}

// A value of "" removes its field with the comma that separated it: an image
// part whole, and an opening's content when no part is left, so the story
// still conforms.
func TestReplaceFileFieldsRemoves(t *testing.T) {
	doc := `{"cover": "a.png", "characters": [{"id": "kai", "avatar": "b.png", "ext": {"x.y": 1}}],
 "scenarios": [{"id": "s", "openings": [
  {"id": "o", "content": [{"type": "image", "src": "c.png"}, {"type": "image", "src": "d.png"}, {"text": "Hi."}, {"type": "image", "src": "e.png"}]},
  {"id": "p", "content": [{"type": "image", "src": "f.png", "ext": {"x.y": 2}}], "ext": {}}]}]}`
	values := map[string]string{
		"/cover":                                "",
		"/characters/0/avatar":                  "",
		"/scenarios/0/openings/0/content/0/src": "",
		"/scenarios/0/openings/0/content/1/src": "",
		"/scenarios/0/openings/0/content/3/src": "https://x.test/e.png",
		"/scenarios/0/openings/1/content/0/src": "",
	}
	want := `{"characters": [{"id": "kai", "ext": {"x.y": 1}}],
 "scenarios": [{"id": "s", "openings": [
  {"id": "o", "content": [{"text": "Hi."}, {"type": "image", "src": "https://x.test/e.png"}]},
  {"id": "p", "ext": {}}]}]}`
	got, err := troupe.ReplaceFileFields([]byte(doc), values)
	if err != nil || string(got) != want {
		t.Errorf("got %s, %v\nwant %s", got, err, want)
	}
}

// ReplaceSessionFileFields fails for a line the session hasn't, a pointer of
// no field on its line, and a player's or a model's message left without
// content, which no session may hold (Format 11).
func TestReplaceSessionFileFieldsErrors(t *testing.T) {
	session := `{"troupe":"0.15","kind":"session","scenario":"s","characters":[{"id":"mara","name":"Mara","avatar":"a.png"}]}
{"id":"1","from":"player","content":[{"type":"image","src":"b.png"}]}
{"id":"2","parent":"1","from":"model","content":[{"type":"image","src":"c.png"},{"type":"image","src":"d.png"}]}
`
	for _, values := range []map[int]map[string]string{
		{4: {"/content/0/src": ""}},
		{0: {"/characters/0/avatar": ""}},
		{1: {"/content/0/src": ""}},
		{2: {"/characters/0/avatar": ""}},
		{3: {"/content/0/src": "", "/content/1/src": ""}},
	} {
		if err := troupe.ReplaceSessionFileFields(io.Discard, strings.NewReader(session), values); err == nil {
			t.Errorf("%v: no error", values)
		}
	}
}
