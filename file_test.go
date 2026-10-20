package troupe_test

import (
	"reflect"
	"slices"
	"testing"

	"github.com/TroupesAI/troupe-go"
)

// FileFields yields every field that holds a file, with its pointer and its
// form, and leaves out values of no form, which a bundle that fails the
// schema can hold.
func TestFileFields(t *testing.T) {
	b := &troupe.Bundle{
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
	if got := slices.Collect(b.FileFields()); !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
}

// ReplaceFileFields fails, rather than writing a document that doesn't
// conform, for a pointer that names no string, or a value that holds no file.
func TestReplaceFileFieldsErrors(t *testing.T) {
	doc := []byte(`{"cover":"a.png","characters":[{"avatar":"b.png"}]}`)
	for _, values := range []map[string]string{
		{"/cover": "c.png", "/avatar": "c.png"},
		{"/characters": "c.png"},
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
