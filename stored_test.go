package troupe_test

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"maps"
	"math/rand/v2"
	"reflect"
	"slices"
	"testing"

	"github.com/TroupesAI/troupe-go"
)

// A state stored and restored is the state it was: it holds the same keys
// and values, and the next message's point evaluates and activates the same.
// Stored, it holds no U+0000, though a value and a session entry do.
func TestStoreState(t *testing.T) {
	lines := append(slices.Clone(activationSession),
		`{"id":"5","parent":"4","from":"model","char":"a","at":"2026-10-17T09:59:00+02:00","content":[{"text":"x"}],"entries":[{"id":"nul","text":"\u0000\u0001","keys":["x"]}],"activated":["s/cool"]}`,
		`{"id":"6","parent":"5","from":"model","char":"b","at":"2026-10-17T10:00:00+02:00","content":[{"text":"x"}],"changes":[{"by":"model","set":{"state.note":"a\u0000b\u0001c\u00010"}}]}`)
	story, s := readStory(t, activationBundle, lines...)
	probes := []string{"turn == 1", "turn == 3", "user == 'p'", "elapsed == 60", "hour == 10", "state.note.size() == 7"}
	seen := map[bool]bool{}
	eval := func(p *troupe.Point) []any {
		var out []any
		for _, expr := range probes {
			got, err := p.Eval(expr, "s", 4)
			if err != nil {
				t.Fatal(err)
			}
			seen[got] = true
			out = append(out, got)
		}
		return append(out, refs(p.Active(rand.New(rand.NewPCG(1, 2))))) //nolint:gosec // G404: a seeded source makes the test reproducible
	}
	before := story.Initial()
	for _, m := range s.Messages {
		data, err := json.Marshal(before)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(data, []byte(`\u0000`)) {
			t.Errorf("before message %s, the stored state holds U+0000: %s", m.ID, data)
		}
		restored, err := story.UnmarshalState(data)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := maps.Collect(restored.All()), maps.Collect(before.All()); !reflect.DeepEqual(got, want) {
			t.Errorf("before message %s, restored %v, want %v", m.ID, got, want)
		}
		recent := []troupe.Message{m}
		if got, want := eval(story.PointAt(restored, recent, "a")), eval(story.PointAt(before, recent, "a")); !reflect.DeepEqual(got, want) {
			t.Errorf("at message %s, restored %v, want %v", m.ID, got, want)
		}
		before = story.Apply(before, &m)
	}
	if !seen[true] || !seen[false] {
		t.Fatal("the probes don't tell points apart")
	}
	if got, _ := before.Get("state.note"); got != "a\x00b\x01c\x010" {
		t.Errorf("state.note is %q", got)
	}

	// A story whose header has gained a joiner restores the state, with the
	// joiner's keys at their initial values.
	header := `{"troupe":"0.14","kind":"session","scenario":"s","characters":[{"id":"p","name":"Pat","description":"Pat."},{"id":"j","name":"Jo","description":"Jo."}],"players":["p"]}`
	joined, _ := readStory(t, activationBundle, header)
	data, err := json.Marshal(before)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := joined.UnmarshalState(data)
	if err != nil {
		t.Fatal(err)
	}
	if trust, _ := restored.Get("state.trust.j"); trust != int64(5) {
		t.Errorf("the joiner's trust is %v", trust)
	}

	for name, edit := range map[string]func(map[string]jsontext.Value){
		"another version": func(v map[string]jsontext.Value) { v["troupe"] = jsontext.Value(`"0.13"`) },
		"an unknown key":  func(v map[string]jsontext.Value) { v["values"] = jsontext.Value(`{"state.missing":1}`) },
		"an invalid value": func(v map[string]jsontext.Value) {
			v["values"] = jsontext.Value(`{"state.n":"two"}`)
		},
	} {
		var v map[string]jsontext.Value
		if err := json.Unmarshal(data, &v); err != nil {
			t.Fatal(err)
		}
		edit(v)
		edited, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := story.UnmarshalState(edited); err == nil {
			t.Errorf("a stored state with %s restores", name)
		}
	}
	if _, err := story.UnmarshalState([]byte(`{"troupe":`)); err == nil {
		t.Error("a stored state that isn't JSON restores")
	}
}
