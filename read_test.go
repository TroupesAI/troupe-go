package troupe_test

import (
	"archive/zip"
	"bytes"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/TroupesAI/troupe-go"
)

// A document that fails the schema still reads, so later checks see it as
// written: a value of the wrong type is absent, not zero, and a list holding
// an item of the wrong type is absent as a whole, so every other item keeps
// its index. What the model holds is what the document says: an integer may
// be written with a zero fraction, a part has only the fields its type
// defines, and ext keeps its bytes as written.
func TestReadBundleModel(t *testing.T) {
	in := `{"troupe":"0.14","kind":"bundle","name":5,"tags":["a",5],"ext":{"x.y": [1, 2.0]},
		"scenarios":[{"id":"s",
			"entries":[{"id":"e","text":"t","scanDepth":8.0,"chance":"often"}],
			"openings":[{"id":"o","set":{"state.v":1,"state.w":null},
				"content":[{"text":"hi","src":"x.png"},{"type":"image","src":"https://x.test/a.png","text":"no"}]}]}]}`
	b, _, err := troupe.ReadBundle(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	eight := 8
	want := &troupe.Bundle{
		Troupe: "0.14",
		Ext:    jsontext.Value(`{"x.y": [1, 2.0]}`),
		Scenarios: []troupe.Scenario{{
			ID:      "s",
			Entries: []troupe.Entry{{ID: "e", Text: "t", ScanDepth: &eight}},
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
}

// A package's sessions are its files anywhere under sessions/, by path, and
// every diagnostic names the file it is in. Its other files, which aren't
// documents, are listed and open by path, and bundle.json is kept as written.
// A session that fails PARSE fails the whole package, which is then reported
// with that code only (Format 15.3).
func TestReadPackage(t *testing.T) {
	header := `{"troupe":"0.14","kind":"session","scenario":"s"}` + "\n"
	message := `{"id":"1","from":"player","content":[{"text":"Hi."}]}`
	bundle := `{"troupe":"0.14","kind":"bundle","mystery":1,"scenarios":[{"id":"s"}]}`
	files := map[string]string{
		"bundle.json":                          bundle,
		"sessions/a.troupe-session.jsonl":      header + message,
		"sessions/old/b.troupe-session.jsonl":  header + `{"id":"1","from":"player","content":[{"text":"Hi."}],"mystery":1}`,
		"sessions/notes.txt":                   "not a session",
		"elsewhere/stray.troupe-session.jsonl": "not a session either",
	}
	read := func() (*troupe.Package, []troupe.Diagnostic) {
		var buf bytes.Buffer
		w := zip.NewWriter(&buf)
		for name, data := range files {
			f, err := w.Create(name)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.Write([]byte(data)); err != nil {
				t.Fatal(err)
			}
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		p, diags, err := troupe.ReadPackage(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
		if err != nil {
			t.Fatal(err)
		}
		return p, diags
	}
	p, diags := read()
	if p == nil {
		t.Fatalf("the package doesn't read: %v", diags)
	}
	if got := slices.Sorted(maps.Keys(p.Sessions)); !slices.Equal(got, []string{"sessions/a.troupe-session.jsonl", "sessions/old/b.troupe-session.jsonl"}) {
		t.Errorf("sessions %q", got)
	}
	if want := []troupe.Diagnostic{
		{Code: troupe.CodeUnknownField, File: "bundle.json", Pointer: "/mystery"},
		{Code: troupe.CodeUnknownField, File: "sessions/old/b.troupe-session.jsonl", Line: 2, Message: "1", Pointer: "/mystery"},
	}; !reflect.DeepEqual(diags, want) {
		t.Errorf("diagnostics %+v, want %+v", diags, want)
	}
	if string(p.BundleJSON) != bundle {
		t.Errorf("bundle.json %q", p.BundleJSON)
	}
	if want := []string{"elsewhere/stray.troupe-session.jsonl", "sessions/notes.txt"}; !slices.Equal(p.Files, want) {
		t.Errorf("files %q, want %q", p.Files, want)
	}
	f, err := p.Open("sessions/notes.txt")
	if err != nil {
		t.Fatal(err)
	}
	if data, err := io.ReadAll(f); err != nil || string(data) != "not a session" {
		t.Errorf("notes.txt %q, %v", data, err)
	}
	if err := f.Close(); err != nil {
		t.Error(err)
	}
	if _, err := p.Open("missing.png"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("opening a missing file: %v", err)
	}

	files["sessions/broken.troupe-session.jsonl"] = header + `{"id":`
	if p, diags := read(); p != nil || !reflect.DeepEqual(diags, []troupe.Diagnostic{
		{Code: troupe.CodeParse, File: "sessions/broken.troupe-session.jsonl", Line: 2, Pointer: "/id"},
	}) {
		t.Errorf("with a broken session: %v, diagnostics %+v", p != nil, diags)
	}
}

// A package's bundle.json, read without the package but with its files'
// paths, reads as ReadPackage reads it: a package path must name one of them.
func TestReadPackageBundle(t *testing.T) {
	for _, c := range []struct {
		doc  string
		want []string
	}{
		{
			`{"troupe":"0.14","kind":"bundle","mystery":1,"cover":"images/cover.png"}`,
			[]string{"UNKNOWN_FIELD bundle.json /mystery"},
		},
		{
			`{"troupe":"0.14","kind":"bundle","characters":[{"id":"kai","name":"Kai","description":"A thief.","avatar":"images/kai.png"}]}`,
			[]string{"PARSE bundle.json /characters/0/avatar"},
		},
	} {
		var buf bytes.Buffer
		w := zip.NewWriter(&buf)
		for name, data := range map[string]string{"bundle.json": c.doc, "images/cover.png": "PNG"} {
			f, err := w.Create(name)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.Write([]byte(data)); err != nil {
				t.Fatal(err)
			}
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		p, pdiags, err := troupe.ReadPackage(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
		if err != nil {
			t.Fatal(err)
		}
		var pb *troupe.Bundle
		if p != nil {
			pb = p.Bundle
		}

		b, diags, err := troupe.ReadPackageBundle(strings.NewReader(c.doc), []string{"images/cover.png"})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(b, pb) {
			t.Errorf("%s: got  %+v\nReadPackage %+v", c.doc, b, pb)
		}
		if !reflect.DeepEqual(diags, pdiags) {
			t.Errorf("%s: diagnostics %+v\nReadPackage %+v", c.doc, diags, pdiags)
		}
		var got []string
		for _, d := range diags {
			got = append(got, string(d.Code)+" "+d.File+" "+d.Location())
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: diagnostics %q, want %q", c.doc, got, c.want)
		}
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
		{"256 members of a scenario's cast", bundle(`"cast":[` + strings.Repeat(`{},`, 255) + `{}]`), false},
		{"257 members of a scenario's cast", bundle(`"cast":[` + strings.Repeat(`{},`, 256) + `{}]`), true},
		{"257 characters a session adds", func() ([]troupe.Diagnostic, error) {
			header := `{"troupe":"0.14","kind":"session","scenario":"s","characters":[` + strings.Join(characters, ",") + `]}`
			_, d, err := troupe.ReadSession(strings.NewReader(header))
			return d, err
		}, true},
		// A limit counts the document as written, items of the wrong type
		// included (Format 15.3).
		{"257 characters a session adds, one not an object", func() ([]troupe.Diagnostic, error) {
			header := `{"troupe":"0.14","kind":"session","scenario":"s","characters":[` + strings.Join(characters[1:], ",") + `,5]}`
			_, d, err := troupe.ReadSession(strings.NewReader(header))
			return d, err
		}, true},
		{"100,001 messages", session(messages...), true},
		{"10,001 session entries on a path", session(
			root,
			`{"id":"a","parent":"r","from":"player","content":[{"text":""}],"entries":[`+entries(6000)+`]}`,
			`{"id":"b","parent":"a","from":"player","content":[{"text":""}],"entries":[`+strings.ReplaceAll(entries(4001), `"e`, `"f`)+`]}`,
		), true},
		// A message without an ID fails the schema, but roots stay roots.
		{"10,000 session entries on each root, one root without an ID", session(
			`{"from":"player","content":[{"text":""}],"entries":[`+entries(6000)+`]}`,
			`{"id":"b","from":"player","content":[{"text":""}],"entries":[`+strings.ReplaceAll(entries(4001), `"e`, `"f`)+`]}`,
		), false},
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
