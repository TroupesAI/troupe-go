package troupe_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strconv"
	"testing"

	"github.com/TroupesAI/troupe-go"
)

// pending lists the tests not expected to pass yet: a test type, or one test
// as "<case path>#<test index>". A pending test that passes fails the run, so
// the list can't go stale.
var pending = map[string]bool{
	"cel":   true,
	"check": true,
	"knows": true,
	"macro": true,
	"state": true,
}

type testCase struct {
	Tests []test `json:"tests"`
}

type test struct {
	Type   string         `json:"type"`
	Name   string         `json:"name"`
	Expect jsontext.Value `json:"expect"`

	// parse
	Kind   string `json:"kind"`
	Text   string `json:"text"`
	Base64 string `json:"base64"`
}

// TestConformance runs the spec's conformance suite (spec/conformance).
func TestConformance(t *testing.T) {
	cases := os.DirFS("spec/conformance/cases")
	paths, err := fs.Glob(cases, "*/*.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("no cases in spec/conformance/cases")
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			data, err := fs.ReadFile(cases, path)
			if err != nil {
				t.Fatal(err)
			}
			var c testCase
			if err := json.Unmarshal(data, &c); err != nil {
				t.Fatal(err)
			}
			for i, tc := range c.Tests {
				name := strconv.Itoa(i)
				if tc.Name != "" {
					name += " " + tc.Name
				}
				t.Run(name, func(t *testing.T) {
					expect, err := decodeExpect(tc)
					if err != nil {
						t.Fatalf("suite error: %v", err)
					}
					err = run(tc, expect)
					switch onList := pending[tc.Type] || pending[fmt.Sprintf("%s#%d", path, i)]; {
					case onList && err != nil:
						t.Skipf("pending: %v", err)
					case onList:
						t.Error("passes, so remove it from the pending list")
					case err != nil:
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
			expect = new(struct {
				CheckError bool `json:"checkError"`
			})
		} else {
			expect = new(bool)
		}
	case "check":
		expect = new(struct {
			Codes    []string `json:"codes"`
			Includes []struct {
				Code string `json:"code"`
				At   string `json:"at"`
			} `json:"includes"`
		})
	case "knows":
		expect = new([]string)
	case "parse":
		expect = new(codesExpect)
	default:
		return nil, fmt.Errorf("unknown test type %q", tc.Type)
	}
	return expect, json.Unmarshal(tc.Expect, expect, json.RejectUnknownMembers(true))
}

type codesExpect struct {
	Codes []string `json:"codes"`
}

// run performs a test's operation and compares the result with its expect
// (conformance README section 3).
func run(tc test, expect any) error {
	switch tc.Type {
	case "parse":
		return runParse(tc, expect.(*codesExpect))
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
