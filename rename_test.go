package troupe_test

import (
	"encoding/json/v2"
	"reflect"
	"slices"
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
		got, err := troupe.PatchBundle([]byte(references), patch)
		if err != nil {
			t.Errorf("%s: %v", c.object, err)
			continue
		}
		var gotJSON, wantJSON any
		if err := json.Unmarshal(got, &gotJSON); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(strings.ReplaceAll(references, c.old, c.id)), &wantJSON); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(gotJSON, wantJSON) {
			t.Errorf("%s: got %s", c.object, got)
		}
	}
}

// RenameID leaves whole, and returns, what it can't rewrite without changing
// what it means: string values that are the ID, literals compared with them,
// literals written with escapes, conditions that don't parse, keys that would
// clash, and names that refer to something else in another scenario using
// them, as written or as renamed.
func TestRenameIDLeaves(t *testing.T) {
	b, _, err := troupe.ReadBundle(strings.NewReader(`{"troupe": "0.14", "kind": "bundle",
		"characters": [
			{"id": "kai", "name": "Kai", "description": "d", "entries": [
				{"id": "a", "text": "t", "when": "char == 'kai' || state.lead == 'kai'"},
				{"id": "b", "text": "t", "when": "char == r'kai'"},
				{"id": "c", "text": "t", "when": "state.kai.nerve >"},
				{"id": "d", "text": "t", "when": "opening == 'dawn'"}],
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
		for _, op := range patch {
			if slices.Contains(left, op.Path) || slices.Contains(left, op.From) {
				t.Errorf("%s: %s %s, which it leaves", c.object, op.Op, op.Path)
			}
		}
	}
}

// RenameID refuses an ID the object can't take (Format 2.3), a pointer that
// names no object with an ID, and an object whose ID another shares, and
// renaming to the same ID changes nothing.
func TestRenameIDErrors(t *testing.T) {
	b, _, err := troupe.ReadBundle(strings.NewReader(`{"troupe": "0.14", "kind": "bundle",
		"characters": [{"id": "kai", "name": "Kai", "description": "d"}, {"id": "mara", "name": "Mara", "description": "d"},
			{"id": "mara", "name": "Mara", "description": "d"}],
		"scenarios": [{"id": "heist", "variables": [{"id": "bell", "type": "int", "description": "d"}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range [][2]string{
		{"/characters/0", "mara"},
		{"/characters/0", "heist"},
		{"/characters/0", "bell"},
		{"/characters/0", "state"},
		{"/characters/0", "in"},
		{"/characters/0", "Kai"},
		{"/scenarios/0/variables/0", "kai"},
		{"/characters/1", "elena"},
		{"/characters/3", "elena"},
		{"/characters", "elena"},
		{"/characters/0/id", "elena"},
		{"/cast/0", "elena"},
	} {
		if patch, _, err := b.RenameID(c[0], c[1]); err == nil {
			t.Errorf("%s to %s: no error, and %v", c[0], c[1], patch)
		}
	}
	if patch, left, err := b.RenameID("/characters/0", "kai"); patch != nil || left != nil || err != nil {
		t.Errorf("to its own ID: %v, %v, %v", patch, left, err)
	}
}
