package troupe_test

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"testing"
)

// pending lists the tests not expected to pass yet: a test type, or one test
// as "<case path>#<test index>". A pending test that passes fails the run, so
// the list can't go stale.
var pending = map[string]bool{
	"cel":   true,
	"check": true,
	"knows": true,
	"macro": true,
	"parse": true,
	"state": true,
}

type testCase struct {
	Tests []test `json:"tests"`
}

type test struct {
	Type   string         `json:"type"`
	Name   string         `json:"name"`
	Expect jsontext.Value `json:"expect"`
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
					if err := decodeExpect(tc); err != nil {
						t.Fatalf("suite error: %v", err)
					}
					err := run(tc)
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
func decodeExpect(tc test) error {
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
		expect = new(struct {
			Codes []string `json:"codes"`
		})
	default:
		return fmt.Errorf("unknown test type %q", tc.Type)
	}
	return json.Unmarshal(tc.Expect, expect, json.RejectUnknownMembers(true))
}

// run performs a test's operation and compares the result with its expect
// (conformance README section 3).
func run(tc test) error {
	return fmt.Errorf("%s tests: %w", tc.Type, errors.ErrUnsupported)
}
