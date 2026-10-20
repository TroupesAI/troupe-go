package troupe_test

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/TroupesAI/troupe-go"
)

// Every occurrence of each ID in references is a reference to it: in cast
// members, worlds, parents, knownBy, speakers, set keys and scene values,
// keyed variables' of and initial, macros, and the names and literals of
// conditions.
const references = `{"troupe": "0.14", "kind": "bundle",
	"characters": [
		{"id": "kai", "name": "Kai", "description": "{{state.kai.nerve}}, {{state.trust.kai}}, {{own.nerve}}.",
			"examples": [[{"speaker": "kai", "text": "Hi."}, {"speaker": "user", "text": "Hey."}]],
			"entries": [{"id": "fear", "text": "Heights.", "knownBy": ["kai", "narrator"],
				"when": "char == 'kai' && self == \"kai\" && 'kai' in scene.present && user != 'kai' && 'kai' in players && char in ['kai', 'mara'] && state.kai.nerve > 1 && has(own.nerve) && state.trust['kai'] > 2 && 'kai' in state.trust && state.ledger.kai == 1 && (hit('café') || char == 'kai' || state . kai .// next\n nerve >= 0)"}],
			"variables": [{"id": "nerve", "type": "int", "description": "{{own.nerve}}"}]},
		{"id": "mara", "name": "Mara", "description": "Mara.",
			"entries": [{"id": "fear", "text": "Rats.", "when": "scene.location == 'vault' || 'vault' in scene.within"}]}],
	"worlds": [{"id": "harbor", "name": "Harbor",
		"locations": [{"id": "vault", "name": "The Vault"}, {"id": "cellar", "name": "Cellar", "parent": "vault"}],
		"entries": [{"id": "storm", "text": "Rain.", "when": "self == 'harbor' && state.harbor.tide > 0"}],
		"variables": [{"id": "tide", "type": "int", "description": "d"}]}],
	"scenarios": [{"id": "heist", "premise": "{{state.bell}}, {{own.bell}}.",
		"cast": [{}, {"character": "kai", "notes": "{{state.trust.kai}}"}, {"character": "mara"}],
		"worlds": ["harbor"],
		"variables": [
			{"id": "bell", "type": "int", "description": "d"},
			{"id": "trust", "type": "int", "of": "cast", "initial": {"kai": 5, "*": 1}, "description": "d"},
			{"id": "ledger", "type": "int", "of": ["kai", "guild"], "initial": {"kai": 2}, "description": "d"},
			{"id": "inventory", "type": "int", "of": "*", "description": "d"}],
		"openings": [{"id": "dawn",
			"set": {"scene.location": "vault", "scene.present": ["kai"], "state.kai.nerve": 2, "state.trust.kai": 3,
				"state.ledger.kai": 1, "state.inventory.kai": 1, "state.bell": 1, "state.harbor.tide": 2},
			"content": [{"speaker": "kai", "knownBy": ["kai"], "text": "{{state.bell}}"}]}],
		"entries": [{"id": "plan", "text": "Go.", "when": "opening == 'dawn' && scenario == 'heist' && self == 'heist' && state.bell > 0"}]}]}`

// RenameID rewrites every reference to what it renames, for each kind of
// object, and leaves nothing.
func TestRenameID(t *testing.T) {
	b, diags, err := troupe.ReadBundle(strings.NewReader(references))
	if err != nil || len(append(diags, troupe.Check(b, nil)...)) > 0 {
		t.Fatal(err, diags, troupe.Check(b, nil))
	}
	for _, c := range []struct{ object, old, id string }{
		{"/characters/0", "kai", "kai_m"},
		{"/characters/0/variables/0", "nerve", "grit"},
		{"/worlds/0", "harbor", "port"},
		{"/worlds/0/locations/0", "vault", "safe"},
		{"/worlds/0/variables/0", "tide", "swell"},
		{"/worlds/0/entries/0", "storm", "gale"},
		{"/scenarios/0", "heist", "job"},
		{"/scenarios/0/openings/0", "dawn", "dusk"},
		{"/scenarios/0/variables/0", "bell", "chime"},
		{"/scenarios/0/variables/1", "trust", "faith"},
	} {
		patch, left, err := b.RenameID(c.object, c.id)
		if err != nil || left != nil {
			t.Errorf("%s: %v, left %v", c.object, err, left)
			continue
		}
		got := apply(t, references, patch)
		var want any
		if err := json.Unmarshal([]byte(strings.ReplaceAll(references, c.old, c.id)), &want); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: %s gives %v", c.object, patch, got)
		}
	}
}

// RenameID leaves whole, and returns, what it can't rewrite without changing
// what it means: string values that are the ID, literals compared with them,
// literals written with escapes, names written between backticks, conditions
// that don't parse, keys that would clash, and names that refer to something
// else in another scenario using them, as written or as renamed.
func TestRenameIDLeaves(t *testing.T) {
	b, _, err := troupe.ReadBundle(strings.NewReader(`{"troupe": "0.14", "kind": "bundle",
		"characters": [
			{"id": "kai", "name": "Kai", "description": "d", "entries": [
				{"id": "a", "text": "t", "when": "char == 'kai' || state.lead == 'kai'"},
				{"id": "b", "text": "t", "when": "char == r'kai'"},
				{"id": "c", "text": "t", "when": "state.kai.nerve >"},
				{"id": "d", "text": "t", "when": "opening == 'dawn'"},
				{"id": "e", "text": "t", "when": "state.` + "`kai`" + `.nerve > 0"}],
				"variables": [{"id": "nerve", "type": "int", "description": "d"},
					{"id": "mood", "type": "enum", "values": ["kai", "calm"], "labels": {"kai": "Kai's"}, "description": "d"}]},
			{"id": "mara", "name": "Mara", "description": "{{state.bell}}"}],
		"scenarios": [
			{"id": "one", "variables": [{"id": "bell", "type": "int", "description": "d"},
				{"id": "lead", "type": "string", "initial": "kai", "description": "d"},
				{"id": "pair", "type": "int", "of": ["kai", "kai_m"], "description": "d"}],
				"openings": [{"id": "dawn", "set": {"state.lead": "kai"}}]},
			{"id": "two", "variables": [{"id": "bell", "type": "int", "description": "d"}], "openings": [{"id": "dawn"}]},
			{"id": "three", "variables": [{"id": "toll", "type": "int", "description": "d"}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		object, id string
		left       []string
	}{
		{"/characters/0", "kai_m", []string{
			"/characters/0/entries/0/when",
			"/characters/0/entries/1/when",
			"/characters/0/entries/2/when",
			"/characters/0/entries/4/when",
			"/characters/0/variables/1/labels/kai",
			"/characters/0/variables/1/values/0",
			"/scenarios/0/openings/0/set/state.lead",
			"/scenarios/0/variables/1/initial",
			"/scenarios/0/variables/2/of/0",
		}},
		{"/scenarios/0/variables/0", "chime", []string{"/characters/1/description"}},
		{"/scenarios/0/openings/0", "dusk", []string{"/characters/0/entries/3/when"}},
		{"/scenarios/0/variables/1", "toll", []string{"/characters/0/entries/0/when"}},
	} {
		patch, left, err := b.RenameID(c.object, c.id)
		if err != nil || !slices.Equal(slices.Sorted(slices.Values(left)), c.left) {
			t.Errorf("%s: %v, left %q, want %q", c.object, err, left, c.left)
		}
		var ops []operation
		if err := json.Unmarshal(patch, &ops); err != nil {
			t.Fatal(err)
		}
		for _, op := range ops {
			if slices.Contains(left, op.Path) || slices.Contains(left, op.From) {
				t.Errorf("%s: %s %s, which it leaves", c.object, op.Op, op.Path)
			}
		}
	}
}

// RenameID refuses an ID the object can't take (Format 2.3), wrapping
// ErrIDTaken for one another object has, a pointer that names no object with
// an ID, and an object whose ID another shares. Renaming to the same ID is an
// empty patch.
func TestRenameIDErrors(t *testing.T) {
	b, _, err := troupe.ReadBundle(strings.NewReader(`{"troupe": "0.14", "kind": "bundle",
		"characters": [{"id": "kai", "name": "Kai", "description": "d"}, {"id": "mara", "name": "Mara", "description": "d"},
			{"id": "mara", "name": "Mara", "description": "d"}],
		"scenarios": [{"id": "heist", "variables": [{"id": "bell", "type": "int", "description": "d"}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		object, id string
		taken      bool
	}{
		{"/characters/0", "mara", true},
		{"/characters/0", "heist", true},
		{"/characters/0", "bell", true},
		{"/scenarios/0/variables/0", "kai", true},
		{"/characters/0", "state", false},
		{"/characters/0", "in", false},
		{"/characters/0", "Kai", false},
		{"/characters/1", "elena", false},
		{"/characters/3", "elena", false},
		{"/characters", "elena", false},
		{"/characters/0/id", "elena", false},
		{"/cast/0", "elena", false},
	} {
		if patch, _, err := b.RenameID(c.object, c.id); err == nil || errors.Is(err, troupe.ErrIDTaken) != c.taken {
			t.Errorf("%s to %s: %v, and %s", c.object, c.id, err, patch)
		}
	}
	if patch, left, err := b.RenameID("/characters/0", "kai"); string(patch) != "[]" || left != nil || err != nil {
		t.Errorf("to its own ID: %s, %v, %v", patch, left, err)
	}
}

// operation is an operation of the JSON Patch RenameID returns.
type operation struct {
	Op    string `json:"op"`
	From  string `json:"from"`
	Path  string `json:"path"`
	Value any    `json:"value"`
}

// apply returns doc with patch applied, failing for an operation other than
// the add, replace and move RenameID writes, or one RFC 6902 can't apply.
func apply(t *testing.T, doc string, patch jsontext.Value) any {
	t.Helper()
	var v any
	var ops []operation
	if err := json.Unmarshal([]byte(doc), &v); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(patch, &ops); err != nil {
		t.Fatal(err)
	}
	// at returns the object or array holding the value at p, and its token.
	at := func(p string) (any, string) {
		tokens := slices.Collect(jsontext.Pointer(p).Tokens())
		parent := v
		for _, tok := range tokens[:len(tokens)-1] {
			switch c := parent.(type) {
			case map[string]any:
				parent = c[tok]
			case []any:
				i, _ := strconv.Atoi(tok)
				parent = c[i]
			}
		}
		return parent, tokens[len(tokens)-1]
	}
	for _, op := range ops {
		value := op.Value
		if op.Op == "move" {
			parent, tok := at(op.From)
			m, _ := parent.(map[string]any)
			var ok bool
			if value, ok = m[tok]; !ok {
				t.Fatalf("%s: from names no member", op.From)
			}
			delete(m, tok)
		}
		parent, tok := at(op.Path)
		switch c := parent.(type) {
		case map[string]any:
			if _, ok := c[tok]; !ok && op.Op == "replace" {
				t.Fatalf("%s: replaces no member", op.Path)
			}
			c[tok] = value
		case []any:
			i, err := strconv.Atoi(tok)
			if err != nil || i >= len(c) || op.Op != "replace" {
				t.Fatalf("%s %s: not a replace of an item", op.Op, op.Path)
			}
			c[i] = value
		default:
			t.Fatalf("%s %s", op.Op, op.Path)
		}
	}
	return v
}
