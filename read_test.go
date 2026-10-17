package troupe_test

import (
	"archive/zip"
	"bytes"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/TroupesAI/troupe-go"
)

// A document that fails the schema still reads, with each value of the wrong
// type absent, so later checks see the document as written.
func TestReadBundleSkipsWrongTypes(t *testing.T) {
	in := `{"troupe":"0.14","kind":"bundle","name":5,"tags":["a",5],"ext":{"x.y": [1, 2.0]},
		"scenarios":[{"id":"s","worlds":[],
			"entries":[{"id":"e","text":"t","scanDepth":8.0,"chance":"often","knownBy":["a"]}],
			"variables":[
				{"id":"v","type":"int","description":"d","of":"cast","min":-1},
				{"id":"w","type":"int","description":"d","of":["a","b"]}],
			"openings":[{"id":"o","set":{"state.v":1,"state.w":null},
				"content":[{"text":"hi","src":"x.png"},{"type":"image","src":"https://x.test/a.png","text":"no"}]}]}]}`
	b, diags, err := troupe.ReadBundle(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	eight, minusOne := 8, int64(-1)
	want := &troupe.Bundle{
		Troupe: "0.14",
		Ext:    jsontext.Value(`{"x.y": [1, 2.0]}`),
		Scenarios: []troupe.Scenario{{
			ID:      "s",
			Worlds:  &[]string{},
			Entries: []troupe.Entry{{ID: "e", Text: "t", ScanDepth: &eight, KnownBy: []string{"a"}}},
			Variables: []troupe.Variable{
				{ID: "v", Type: "int", Description: "d", Of: troupe.Keys{Cast: true}, Min: &minusOne},
				{ID: "w", Type: "int", Description: "d", Of: troupe.Keys{List: []string{"a", "b"}}},
			},
			Openings: []troupe.Opening{{
				ID:  "o",
				Set: map[string]jsontext.Value{"state.v": jsontext.Value("1")},
				Content: []troupe.Part{
					{Text: "hi"},
					{Type: "image", Src: "https://x.test/a.png"},
				},
			}},
		}},
	}
	if !reflect.DeepEqual(b, want) {
		t.Errorf("got  %+v\nwant %+v", b, want)
	}
	if got := locations(diags); !reflect.DeepEqual(got, []string{
		"SCHEMA /name",
		"SCHEMA /scenarios/0/entries/0/chance",
		"UNKNOWN_FIELD /scenarios/0/openings/0/content/0/src",
		"UNKNOWN_FIELD /scenarios/0/openings/0/content/1/text",
		"SCHEMA /scenarios/0/openings/0/set/state.w",
		"SCHEMA /tags/1",
	}) {
		t.Errorf("diagnostics %q", got)
	}
}

// Reading stops at each limit of Format 14 that the conformance suite has no
// case for, so hostile input can't make it do unbounded work.
func TestReadLimits(t *testing.T) {
	entries := func(n int) string {
		es := make([]string, n)
		for i := range es {
			es[i] = fmt.Sprintf(`{"id":"e%d","text":""}`, i)
		}
		return strings.Join(es, ",")
	}
	bundle := func(scenario string) func() ([]troupe.Diagnostic, error) {
		return func() ([]troupe.Diagnostic, error) {
			_, d, err := troupe.ReadBundle(strings.NewReader(`{"troupe":"0.14","kind":"bundle","scenarios":[{"id":"s",` + scenario + `}]}`))
			return d, err
		}
	}
	session := func(lines ...string) func() ([]troupe.Diagnostic, error) {
		return func() ([]troupe.Diagnostic, error) {
			header := `{"troupe":"0.14","kind":"session","scenario":"s"}`
			_, d, err := troupe.ReadSession(strings.NewReader(header + "\n" + strings.Join(lines, "\n")))
			return d, err
		}
	}
	messages := make([]string, 100_001)
	for i := range messages {
		messages[i] = fmt.Sprintf(`{"id":"m%d","from":"player","content":[{"text":""}]}`, i)
	}
	characters := make([]string, 257)
	for i := range characters {
		characters[i] = fmt.Sprintf(`{"id":"c%d","name":"C","description":""}`, i)
	}
	variables := make([]string, 1001)
	for i := range variables {
		variables[i] = fmt.Sprintf(`{"id":"v%d","type":"bool","description":"d"}`, i)
	}
	root := `{"id":"r","from":"player","content":[{"text":""}]}`
	tests := []struct {
		name  string
		read  func() ([]troupe.Diagnostic, error)
		parse bool
	}{
		{"a bundle over 32 MiB", func() ([]troupe.Diagnostic, error) {
			_, d, err := troupe.ReadBundle(io.MultiReader(strings.NewReader(`{"troupe":"0.14","kind":"bundle"}`), spaces{}))
			return d, err
		}, true},
		{"10,000 entries", bundle(`"entries":[` + entries(10_000) + `]`), false},
		{"10,001 entries", bundle(`"entries":[` + entries(10_001) + `]`), true},
		{"1,001 variables", bundle(`"variables":[` + strings.Join(variables, ",") + `]`), true},
		{"a text over 1 MiB", bundle(`"entries":[{"id":"e","text":"` + strings.Repeat("a", 1<<20+1) + `"}]`), true},
		{"257 characters a session adds", func() ([]troupe.Diagnostic, error) {
			header := `{"troupe":"0.14","kind":"session","scenario":"s","characters":[` + strings.Join(characters, ",") + `]}`
			_, d, err := troupe.ReadSession(strings.NewReader(header))
			return d, err
		}, true},
		{"100,001 messages", session(messages...), true},
		{"10,001 session entries on a path", session(
			root,
			`{"id":"a","parent":"r","from":"player","content":[{"text":""}],"entries":[`+entries(6000)+`]}`,
			`{"id":"b","parent":"a","from":"player","content":[{"text":""}],"entries":[`+strings.ReplaceAll(entries(4001), `"e`, `"f`)+`]}`,
		), true},
		{"10,000 session entries on each branch", session(
			root,
			`{"id":"a","parent":"r","from":"player","content":[{"text":""}],"entries":[`+entries(6000)+`]}`,
			`{"id":"b","parent":"r","from":"player","content":[{"text":""}],"entries":[`+strings.ReplaceAll(entries(6000), `"e`, `"f`)+`]}`,
			// The latest entry per ID counts once.
			`{"id":"c","parent":"b","from":"player","content":[{"text":""}],"entries":[`+strings.ReplaceAll(entries(4000), `"e`, `"f`)+`]}`,
		), false},
		{"10,001 ZIP entries", pkg(func(w *zip.Writer) {
			for i := range 10_001 {
				if _, err := w.Create(fmt.Sprintf("f%d", i)); err != nil {
					panic(err)
				}
			}
		}), true},
		{"an unpacked size over 512 MiB", pkg(func(w *zip.Writer) {
			h := &zip.FileHeader{Name: "big", Method: zip.Store, UncompressedSize64: 512<<20 + 1}
			if _, err := w.CreateRaw(h); err != nil {
				panic(err)
			}
		}), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags, err := tt.read()
			if err != nil {
				t.Fatal(err)
			}
			if parse := len(diags) > 0 && diags[0].Code == troupe.CodeParse; parse != tt.parse || len(diags) > 1 {
				t.Errorf("diagnostics %q", locations(diags))
			}
		})
	}
}

// A reader that fails is an error, not a PARSE.
func TestReadErrors(t *testing.T) {
	fail := errors.New("broken")
	if _, _, err := troupe.ReadBundle(io.MultiReader(strings.NewReader("{"), failing{fail})); !errors.Is(err, fail) {
		t.Errorf("ReadBundle: %v", err)
	}
	if _, _, err := troupe.ReadSession(failing{fail}); !errors.Is(err, fail) {
		t.Errorf("ReadSession: %v", err)
	}
	if _, _, err := troupe.ReadPackage(failing{fail}, 100); !errors.Is(err, fail) {
		t.Errorf("ReadPackage: %v", err)
	}
}

func locations(diags []troupe.Diagnostic) []string {
	var out []string
	for _, d := range diags {
		out = append(out, string(d.Code)+" "+d.Location())
	}
	return out
}

// pkg returns a reading of the package that write makes, with a bundle.json.
func pkg(write func(*zip.Writer)) func() ([]troupe.Diagnostic, error) {
	return func() ([]troupe.Diagnostic, error) {
		var buf bytes.Buffer
		w := zip.NewWriter(&buf)
		f, err := w.Create("bundle.json")
		if err != nil {
			return nil, err
		}
		if _, err := f.Write([]byte(`{"troupe":"0.14","kind":"bundle"}`)); err != nil {
			return nil, err
		}
		write(w)
		if err := w.Close(); err != nil {
			return nil, err
		}
		_, d, err := troupe.ReadPackage(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
		return d, err
	}
}

// spaces is endless JSON whitespace.
type spaces struct{}

func (spaces) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = ' '
	}
	return len(p), nil
}

type failing struct{ err error }

func (f failing) Read([]byte) (int, error)          { return 0, f.err }
func (f failing) ReadAt([]byte, int64) (int, error) { return 0, f.err }
