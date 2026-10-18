package troupe_test

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"strings"
	"testing"

	"github.com/TroupesAI/troupe-go"
)

// PatchBundle keeps every byte a patch doesn't change, ext's escapes
// included, and writes what it adds compactly, spaced like its neighbors. It
// reads only what operations reach, so a value of the wrong type is patched
// like any other, and a test compares numbers by value and objects without
// order.
func TestPatchBundle(t *testing.T) {
	doc := "{\n  \"troupe\": \"0.14\",\n  \"kind\": \"bundle\",\n  \"tags\": [\n    \"a\",\n    \"b\"\n  ],\n" +
		"  \"ext\": {\"x.y\": {\"k\" : [1 , 2], \"s\": \"\\u00e9\"}}\n}\n"
	for _, c := range []struct{ patch, want string }{
		{
			`[{"op": "test", "path": "/ext/x.y", "value": {"s": "é", "k": [1.0, 2e0]}},
			  {"op": "add", "path": "/tags/0", "value": "z"},
			  {"op": "remove", "path": "/tags/2"},
			  {"op": "add", "path": "/name", "value": { "en" : "N" }},
			  {"op": "replace", "path": "/kind", "value": 5},
			  {"op": "move", "from": "/tags/0", "path": "/ext/x.y/k/1"}]`,
			"{\n  \"troupe\": \"0.14\",\n  \"kind\": 5,\n  \"tags\": [\n    \"a\"\n  ],\n" +
				"  \"ext\": {\"x.y\": {\"k\" : [1, \"z\" , 2], \"s\": \"\\u00e9\"}},\n  \"name\": {\"en\":\"N\"}\n}\n",
		},
		{
			`[{"op": "remove", "path": "/troupe"},
			  {"op": "add", "path": "/tags/-", "value": "c"},
			  {"op": "move", "from": "/tags/1", "path": "/tags/0"},
			  {"op": "add", "path": "/ext/x.y/k/-", "value": 3},
			  {"op": "add", "path": "/ext/new", "value": []},
			  {"op": "add", "path": "/ext/new/0", "value": {}},
			  {"op": "add", "path": "/ext/new/0/a", "value": true}]`,
			"{\n  \"kind\": \"bundle\",\n  \"tags\": [\n    \"b\",\n    \"a\",\n    \"c\"\n  ],\n" +
				"  \"ext\": {\"x.y\": {\"k\" : [1 , 2, 3], \"s\": \"\\u00e9\"},\"new\": [{\"a\":true}]}\n}\n",
		},
		{`[{"op": "replace", "path": "", "value": {"kind": "bundle"}}]`, "{\"kind\":\"bundle\"}\n"},
	} {
		var patch []troupe.Operation
		if err := json.Unmarshal([]byte(c.patch), &patch); err != nil {
			t.Fatal(err)
		}
		got, err := troupe.PatchBundle([]byte(doc), patch)
		if err != nil {
			t.Errorf("%s: %v", c.patch, err)
		} else if string(got) != c.want {
			t.Errorf("%s:\ngot  %q\nwant %q", c.patch, got, c.want)
		}
	}
}

// PatchBundle applies all of a patch or none of it. It refuses what RFC 6902
// does, values that aren't I-JSON or are nested too deep, and a bundle over
// 32 MiB (Format 2.2, 14), and its error wraps ErrTestFailed exactly when a
// test fails.
func TestPatchBundleErrors(t *testing.T) {
	doc := []byte(`{"kind":"bundle","tags":["a"],"ext":{}}`)
	// 62 arrays inside ext and the bundle are as deep as values nest.
	deep := strings.Repeat("[", 62) + strings.Repeat("]", 62)
	op := func(op, path, value string) troupe.Operation {
		o := troupe.Operation{Op: op, Path: path}
		if value != "" {
			o.Value = jsontext.Value(value)
		}
		return o
	}
	for _, c := range []struct {
		patch []troupe.Operation
		test  bool
	}{
		{[]troupe.Operation{op("add", "/x", "1"), op("test", "/tags", `["b"]`)}, true},
		{[]troupe.Operation{op("test", "/tags/0", `"b"`)}, true},
		{[]troupe.Operation{op("test", "/tags", `["a", "b"]`)}, true},
		{[]troupe.Operation{op("test", "/ext", `{"a": 1}`)}, true},
		{[]troupe.Operation{op("test", "/tags/1", `"a"`)}, false},
		{[]troupe.Operation{{Op: "copy", From: "/tags", Path: "/x"}}, false},
		{[]troupe.Operation{op("remove", "/tags/1", "")}, false},
		{[]troupe.Operation{op("remove", "", "")}, false},
		{[]troupe.Operation{op("add", "/tags/2", `"b"`)}, false},
		{[]troupe.Operation{op("add", "/tags/01", `"b"`)}, false},
		{[]troupe.Operation{op("add", "/kind/x", `"b"`)}, false},
		{[]troupe.Operation{op("replace", "/name", `"N"`)}, false},
		{[]troupe.Operation{op("add", "/x", "")}, false},
		{[]troupe.Operation{op("add", "x", "1")}, false},
		{[]troupe.Operation{op("add", "/x", `"\uFFFF"`)}, false},
		{[]troupe.Operation{op("add", "/x", `{"a":1,"a":2}`)}, false},
		{[]troupe.Operation{op("add", "/ext/x", strings.Repeat("[", 63)+strings.Repeat("]", 63))}, false},
		{[]troupe.Operation{op("add", "/ext/x", deep), op("add", "/ext/y", "[]"), {Op: "move", From: "/ext/x", Path: "/ext/y/0"}}, false},
		{[]troupe.Operation{op("add", "/x", `"`+strings.Repeat("a", 32<<20)+`"`)}, false},
	} {
		got, err := troupe.PatchBundle(doc, c.patch)
		if err == nil {
			t.Errorf("%.80v: no error, and %.80s", c.patch, got)
		} else if errors.Is(err, troupe.ErrTestFailed) != c.test {
			t.Errorf("%.80v: %v", c.patch, err)
		}
	}
	if _, err := troupe.PatchBundle(doc, []troupe.Operation{op("add", "/ext/x", deep)}); err != nil {
		t.Error(err)
	}
	for _, doc := range []string{`{"kind":`, `{"a":1,"a":2}`, `{} {}`} {
		if _, err := troupe.PatchBundle([]byte(doc), nil); err == nil {
			t.Errorf("%s: no error", doc)
		}
	}
}
