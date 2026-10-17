package troupe_test

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"math/rand/v2"
	"os"
	"path"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/TroupesAI/troupe-go"
)

// spec holds the copy of the spec repository's files: the cases are in
// conformance/cases, and the files they name are relative to theirs.
var spec = os.DirFS("spec")

type testCase struct {
	Bundle     jsontext.Value `json:"bundle"`
	BundleFile string         `json:"bundleFile"`
	Session    *struct {
		Header   jsontext.Value   `json:"header"`
		Messages []jsontext.Value `json:"messages"`
	} `json:"session"`
	SessionFile string `json:"sessionFile"`
	Tests       []test `json:"tests"`

	path string // relative to conformance/cases
}

type test struct {
	Type   string         `json:"type"`
	Name   string         `json:"name"`
	Expect jsontext.Value `json:"expect"`

	// state, macro and cel
	At     string   `json:"at"`
	Absent []string `json:"absent"`

	// cel
	Expr      string `json:"expr"`
	Self      string `json:"self"`
	ScanDepth *int   `json:"scanDepth"`

	// macro, and char for cel
	Text     string `json:"text"`
	Char     string `json:"char"`
	Owner    string `json:"owner"`
	Narrator bool   `json:"narrator"`

	// knows, and macro, whose part is an object of message and index
	Message string         `json:"message"`
	Part    jsontext.Value `json:"part"`

	// parse, and text as for macro
	Kind   string `json:"kind"`
	Base64 string `json:"base64"`
}

// TestConformance runs the spec's conformance suite (spec/conformance).
func TestConformance(t *testing.T) {
	cases, err := fs.Sub(spec, "conformance/cases")
	if err != nil {
		t.Fatal(err)
	}
	paths, err := fs.Glob(cases, "*/*.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("no cases in spec/conformance/cases")
	}
	for _, name := range paths {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			data, err := fs.ReadFile(cases, name)
			if err != nil {
				t.Fatal(err)
			}
			c := testCase{path: name}
			if err := json.Unmarshal(data, &c); err != nil {
				t.Fatal(err)
			}
			for i, tc := range c.Tests {
				test := strconv.Itoa(i)
				if tc.Name != "" {
					test += " " + tc.Name
				}
				t.Run(test, func(t *testing.T) {
					expect, err := decodeExpect(tc)
					if err != nil {
						t.Fatalf("suite error: %v", err)
					}
					if err := run(&c, tc, expect); err != nil {
						t.Error(err)
					}
				})
			}
		})
	}
}

// decodeExpect decodes a test's expect, failing on fields the conformance
// README doesn't define for its type (section 3).
func decodeExpect(tc test) (any, error) {
	var expect any
	switch tc.Type {
	case "state":
		expect = new(map[string]jsontext.Value)
	case "macro":
		expect = new(string)
	case "cel":
		if tc.Expect.Kind() == '{' {
			expect = new(celError)
		} else {
			expect = new(bool)
		}
	case "check":
		expect = new(checkExpect)
	case "knows":
		expect = new([]string)
	case "parse":
		expect = new(codesExpect)
	default:
		return nil, fmt.Errorf("unknown test type %q", tc.Type)
	}
	return expect, json.Unmarshal(tc.Expect, expect, json.RejectUnknownMembers(true))
}

type celError struct {
	CheckError bool `json:"checkError"`
}

type codesExpect struct {
	Codes []string `json:"codes"`
}

type checkExpect struct {
	Codes    []string `json:"codes"`
	Includes []struct {
		Code string `json:"code"`
		At   string `json:"at"`
	} `json:"includes"`
}

// run performs a test's operation and compares the result with its expect
// (conformance README section 3).
func run(c *testCase, tc test, expect any) error {
	switch tc.Type {
	case "parse":
		return runParse(tc, expect.(*codesExpect))
	case "check":
		return runCheck(c, expect.(*checkExpect))
	case "state":
		return runState(c, tc, *expect.(*map[string]jsontext.Value))
	case "knows":
		return runKnows(c, tc, *expect.(*[]string))
	case "macro":
		return runMacro(c, tc, *expect.(*string))
	case "cel":
		return runCel(c, tc, expect)
	}
	return fmt.Errorf("%s tests: %w", tc.Type, errors.ErrUnsupported)
}

// runParse reads the input as a session reader (Format 15.2), so tests marked
// "level": "session" run too (conformance README section 3.6).
func runParse(tc test, expect *codesExpect) error {
	input := []byte(tc.Text)
	if tc.Base64 != "" {
		var err error
		if input, err = base64.StdEncoding.DecodeString(tc.Base64); err != nil {
			return err
		}
	}
	var diags []troupe.Diagnostic
	var err error
	switch tc.Kind {
	case "bundle":
		_, diags, err = troupe.ReadBundle(bytes.NewReader(input))
	case "session":
		_, diags, err = troupe.ReadSession(bytes.NewReader(input))
	case "package":
		_, diags, err = troupe.ReadPackage(bytes.NewReader(input), int64(len(input)))
	default:
		return fmt.Errorf("unknown kind %q", tc.Kind)
	}
	if err != nil {
		return err
	}
	return compareCodes(diags, expect.Codes)
}

// runCheck validates the case's documents as reading them does, then runs
// every other check (conformance README section 3.4).
func runCheck(c *testCase, expect *checkExpect) error {
	b, s, diags, err := c.read()
	if err != nil {
		return err
	}
	diags = append(diags, troupe.Check(b, s)...)
	if err := compareCodes(diags, expect.Codes); err != nil {
		return err
	}
	for _, want := range expect.Includes {
		if !slices.ContainsFunc(diags, func(d troupe.Diagnostic) bool {
			return string(d.Code) == want.Code && d.Location() == want.At
		}) {
			return fmt.Errorf("no %s at %s; diagnostics %v", want.Code, want.At, diags)
		}
	}
	return nil
}

// runState compares the state at a message, flattened, with expect, a
// subset of it, and checks that the keys absent lists aren't in it
// (conformance README section 3.1).
func runState(c *testCase, tc test, expect map[string]jsontext.Value) error {
	story, _, err := c.story()
	if err != nil {
		return err
	}
	state := story.Initial()
	if tc.At != "" {
		if state, err = story.StateAt(tc.At); err != nil {
			return err
		}
	}
	flat := maps.Collect(state.All())
	for key, want := range expect {
		got, ok := flat[key]
		if !ok {
			return fmt.Errorf("%s isn't in the state %v", key, flat)
		}
		equal, err := sameJSON(got, want)
		if err != nil {
			return err
		}
		if !equal {
			return fmt.Errorf("%s is %v, want %s", key, got, want)
		}
	}
	for _, key := range tc.Absent {
		if _, ok := flat[key]; ok {
			return fmt.Errorf("%s is in the state", key)
		}
	}
	return nil
}

// runKnows compares who knows a part of a message with expect (conformance
// README section 3.5).
func runKnows(c *testCase, tc test, expect []string) error {
	story, s, err := c.story()
	if err != nil {
		return err
	}
	i := slices.IndexFunc(s.Messages, func(m troupe.Message) bool { return m.ID == tc.Message })
	if i < 0 {
		return fmt.Errorf("no message %s", tc.Message)
	}
	m := &s.Messages[i]
	before := story.Initial()
	if m.Parent != "" {
		if before, err = story.StateAt(m.Parent); err != nil {
			return err
		}
	}
	var part int
	if err := json.Unmarshal(tc.Part, &part); err != nil {
		return err
	}
	if got := story.KnownBy(m, before)[part]; !slices.Equal(got, expect) {
		return fmt.Errorf("known by %q, want %q", got, expect)
	}
	return nil
}

// runMacro compares rendered text with expect: the test's text, or a
// message part's as it is shown (conformance README section 3.2). Picks in
// these tests have one option or are chosen already, so any source will do.
func runMacro(c *testCase, tc test, expect string) error {
	story, s, err := c.story()
	if err != nil {
		return err
	}
	r := rand.New(rand.NewPCG(0, 0)) //nolint:gosec // G404: a seeded source makes the test reproducible
	var got string
	if tc.Part != nil {
		var part struct {
			Message string `json:"message"`
			Index   int    `json:"index"`
		}
		if err := json.Unmarshal(tc.Part, &part, json.RejectUnknownMembers(true)); err != nil {
			return err
		}
		i := slices.IndexFunc(s.Messages, func(m troupe.Message) bool { return m.ID == part.Message })
		if i < 0 {
			return fmt.Errorf("no message %s", part.Message)
		}
		at, err := story.StateAt(part.Message)
		if err != nil {
			return err
		}
		got = story.RenderParts(&s.Messages[i], at, r)[part.Index].Text
	} else {
		ctx := troupe.Context{Char: tc.Char, Owner: tc.Owner, Narrator: tc.Narrator, State: story.Initial()}
		if tc.At != "" {
			if ctx.State, err = story.StateAt(tc.At); err != nil {
				return err
			}
		}
		got = story.Render(tc.Text, ctx, r)
	}
	if got != expect {
		return fmt.Errorf("rendered %q, want %q", got, expect)
	}
	return nil
}

// runCel evaluates a condition at the evaluation point at, for char, owned by
// self and scanning scanDepth messages, and compares the result with expect:
// a bool, or a CEL_ERROR (conformance README section 3.3).
func runCel(c *testCase, tc test, expect any) error {
	story, s, err := c.story()
	if err != nil {
		return err
	}
	self, scanDepth := tc.Self, 4
	if self == "" {
		self = c.scenario(s)
	}
	if tc.ScanDepth != nil {
		scanDepth = *tc.ScanDepth
	}
	var path []troupe.Message
	if tc.At != "" {
		if path, err = pathTo(s, tc.At); err != nil {
			return err
		}
	}
	got, err := story.PointAt(story.Initial(), path, tc.Char).Eval(tc.Expr, self, scanDepth)
	if want, ok := expect.(*celError); ok {
		if err == nil || !want.CheckError {
			return fmt.Errorf("%s compiles, want a CEL_ERROR", tc.Expr)
		}
		return nil
	}
	if err != nil {
		return err
	}
	if want := *expect.(*bool); got != want {
		return fmt.Errorf("%s is %v, want %v", tc.Expr, got, want)
	}
	return nil
}

// pathTo returns the messages of s on the path from a root to id.
func pathTo(s *troupe.Session, id string) ([]troupe.Message, error) {
	var path []troupe.Message
	for id != "" {
		i := slices.IndexFunc(s.Messages, func(m troupe.Message) bool { return m.ID == id })
		if i < 0 {
			return nil, fmt.Errorf("no message %s", id)
		}
		path = append(path, s.Messages[i])
		id = s.Messages[i].Parent
	}
	slices.Reverse(path)
	return path, nil
}

// scenario returns the ID of the scenario the story plays: the session's, or
// without one the bundle's first (conformance README section 2).
func (c *testCase) scenario(s *troupe.Session) string {
	if s != nil {
		return s.Header.Scenario
	}
	b, _, _, _ := c.read()
	return b.Scenarios[0].ID
}

// sameJSON reports whether got, encoded as JSON, equals want, comparing
// numbers as doubles (conformance README section 3).
func sameJSON(got any, want jsontext.Value) (bool, error) {
	data, err := json.Marshal(got)
	if err != nil {
		return false, err
	}
	var g, w any
	if err := json.Unmarshal(data, &g); err != nil {
		return false, err
	}
	if err := json.Unmarshal(want, &w); err != nil {
		return false, err
	}
	return reflect.DeepEqual(g, w), nil
}

// story returns the story of the case's bundle and session (conformance
// README section 2), and the session.
func (c *testCase) story() (*troupe.Story, *troupe.Session, error) {
	b, s, _, err := c.read()
	if err != nil {
		return nil, nil, err
	}
	story, err := troupe.NewStory(b, s)
	return story, s, err
}

// read reads a fresh copy of the case's bundle and session, as conformance
// README section 1 describes them, with the diagnostics of reading them.
func (c *testCase) read() (*troupe.Bundle, *troupe.Session, []troupe.Diagnostic, error) {
	if path.Base(c.BundleFile) == "bundle.json" {
		b, s, diags, err := c.readPackage()
		if err != nil || s != nil || c.Session == nil {
			return b, s, diags, err
		}
		s, sdiags, err := c.readSession()
		return b, s, append(diags, sdiags...), err
	}
	var bundle []byte
	var err error
	switch {
	case c.BundleFile != "":
		bundle, err = fs.ReadFile(spec, c.file(c.BundleFile))
	case c.Bundle != nil:
		bundle, err = withVersion(c.Bundle)
	default:
		bundle, err = withVersion(jsontext.Value(`{"kind":"bundle"}`))
	}
	if err != nil {
		return nil, nil, nil, err
	}
	b, diags, err := troupe.ReadBundle(bytes.NewReader(bundle))
	if err != nil {
		return nil, nil, nil, err
	}
	if b == nil {
		return nil, nil, nil, fmt.Errorf("the bundle doesn't read: %v", diags)
	}
	if c.SessionFile == "" && c.Session == nil {
		return b, nil, diags, nil
	}
	s, sdiags, err := c.readSession()
	return b, s, append(diags, sdiags...), err
}

// readSession reads the case's session, from its file or as given inline.
func (c *testCase) readSession() (*troupe.Session, []troupe.Diagnostic, error) {
	var session []byte
	var err error
	switch {
	case c.SessionFile != "":
		session, err = fs.ReadFile(spec, c.file(c.SessionFile))
	default:
		var header []byte
		if header, err = withVersion(c.Session.Header); err != nil {
			return nil, nil, err
		}
		lines := [][]byte{header}
		for _, m := range c.Session.Messages {
			line := slices.Clone(m)
			if err := line.Compact(); err != nil {
				return nil, nil, err
			}
			lines = append(lines, line)
		}
		session = bytes.Join(lines, []byte("\n"))
	}
	if err != nil {
		return nil, nil, err
	}
	s, diags, err := troupe.ReadSession(bytes.NewReader(session))
	if err != nil {
		return nil, nil, err
	}
	if s == nil {
		return nil, nil, fmt.Errorf("the session doesn't read: %v", diags)
	}
	return s, diags, nil
}

// readPackage reads the package whose bundle.json the case names, and the
// session of it the case names (conformance README section 1).
func (c *testCase) readPackage() (*troupe.Bundle, *troupe.Session, []troupe.Diagnostic, error) {
	dir := path.Dir(c.file(c.BundleFile))
	files, err := fs.Sub(spec, dir)
	if err != nil {
		return nil, nil, nil, err
	}
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	if err := w.AddFS(files); err != nil {
		return nil, nil, nil, err
	}
	if err := w.Close(); err != nil {
		return nil, nil, nil, err
	}
	p, diags, err := troupe.ReadPackage(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		return nil, nil, nil, err
	}
	if p == nil {
		return nil, nil, nil, fmt.Errorf("the package doesn't read: %v", diags)
	}
	if c.SessionFile == "" {
		return p.Bundle, nil, diags, nil
	}
	name := strings.TrimPrefix(c.file(c.SessionFile), dir+"/")
	s := p.Sessions[name]
	if s == nil {
		return nil, nil, nil, fmt.Errorf("no session %s in the package", name)
	}
	return p.Bundle, s, diags, nil
}

// file returns the path within spec of a file the case names.
func (c *testCase) file(name string) string {
	return path.Join("conformance/cases", path.Dir(c.path), name)
}

// withVersion returns the document doc with troupe set to the spec version,
// as the runner sets it before anything else (conformance README section 1).
func withVersion(doc jsontext.Value) ([]byte, error) {
	var members map[string]jsontext.Value
	if err := json.Unmarshal(doc, &members); err != nil {
		return nil, err
	}
	members["troupe"] = jsontext.Value(strconv.Quote(troupe.Version))
	return json.Marshal(members)
}

// compareCodes compares the sorted, distinct codes of diags with want.
func compareCodes(diags []troupe.Diagnostic, want []string) error {
	got := []string{}
	for _, d := range diags {
		got = append(got, string(d.Code))
	}
	slices.Sort(got)
	got = slices.Compact(got)
	if !slices.Equal(got, want) {
		return fmt.Errorf("codes %v, want %v; diagnostics %v", got, want, diags)
	}
	return nil
}
