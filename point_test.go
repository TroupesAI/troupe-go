package troupe_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/TroupesAI/troupe-go"
)

// A point built from the last messages of a path, and the state before them,
// is the point built from the whole path, so a caller that keeps states needs
// only the messages a scan reaches: here the last that isn't hidden.
func TestPointAtRecent(t *testing.T) {
	b, diags, err := troupe.ReadBundle(strings.NewReader(`{"troupe":"0.14","kind":"bundle",
		"characters":[{"id":"a","name":"Ann","description":"Ann."}],
		"scenarios":[{"id":"s","cast":[{},{},{"character":"a"}],
			"variables":[{"id":"n","type":"int","by":["player"],"description":"d"}],
			"openings":[{"id":"o","content":[{"text":"The lamp is lit."}]}]}]}`))
	if err != nil || diags != nil {
		t.Fatal(err, diags)
	}
	s, diags, err := troupe.ReadSession(strings.NewReader(strings.Join([]string{
		`{"troupe":"0.14","kind":"session","scenario":"s","characters":[{"id":"p","name":"Pat","description":"Pat."},{"id":"q","name":"Quin","description":"Quin."}],"players":["p","q"]}`,
		`{"id":"1","opening":"o","from":"system","content":[{"text":"The lamp is lit."}]}`,
		`{"id":"2","parent":"1","from":"player","at":"2026-10-17T10:00:00Z","content":[{"speaker":"q","text":"A rope."}],"changes":[{"by":"player","set":{"state.n":1}}]}`,
		`{"id":"3","parent":"2","from":"model","char":"a","at":"2026-10-17T10:01:00Z","content":[{"speaker":"a","text":"A key."}]}`,
		`{"id":"4","parent":"3","from":"player","hidden":true,"content":[{"speaker":"p","text":"A secret."}]}`,
	}, "\n")))
	if err != nil || diags != nil {
		t.Fatal(err, diags)
	}
	story, err := troupe.NewStory(b, s)
	if err != nil {
		t.Fatal(err)
	}
	probes := []string{
		"turn == 2", "opening == 'o'", "elapsed == 0", "hour == -1", "user == 'q'", "state.n == 1",
		"hit('lamp')", "hit('rope')", "hit('key')", "hit('secret')",
	}
	eval := func(p *troupe.Point) []bool {
		var out []bool
		for _, expr := range probes {
			got, err := p.Eval(expr, "s", 1)
			if err != nil {
				t.Fatal(err)
			}
			out = append(out, got)
		}
		return out
	}
	path := s.Messages
	want := eval(story.PointAt(story.Initial(), path, "a"))
	if !slices.Contains(want, true) || !slices.Contains(want, false) {
		t.Fatalf("the probes don't tell points apart: %v", want)
	}
	before := story.Initial()
	for k := range path[:3] {
		if got := eval(story.PointAt(before, path[k:], "a")); !slices.Equal(got, want) {
			t.Errorf("from message %s: %v, want %v", path[k].ID, got, want)
		}
		before = story.Apply(before, &path[k])
	}
}

// Format 10.2's rules hold for own as for state: selecting char or user from
// a keyed variable is a CEL_ERROR. A field written between backticks is the
// field as written, so one holding a dot names no variable. The error holds
// no text of the condition, which apps log.
func TestConditionRules(t *testing.T) {
	b, diags, err := troupe.ReadBundle(strings.NewReader(`{"troupe":"0.14","kind":"bundle",
		"characters":[{"id":"kai","name":"Kai","description":"d",
			"variables":[{"id":"nerve","type":"int","initial":1,"description":"d"},
				{"id":"feelings","type":"int","of":"cast","description":"d"}]}],
		"scenarios":[{"id":"s","cast":[{"character":"kai"}]}]}`))
	if err != nil || diags != nil {
		t.Fatal(err, diags)
	}
	story, err := troupe.NewStory(b, nil)
	if err != nil {
		t.Fatal(err)
	}
	p := story.PointAt(story.Initial(), nil, "kai")
	for _, c := range []struct {
		expr, self string
		valid      bool
	}{
		{"own.feelings.char == 0", "kai", false},
		{"own.feelings.user == 0", "kai", false},
		{"own.feelings[char] == 0", "kai", true},
		{"state.`kai`.nerve == 1", "s", true},
		{"state.`kai.nerve` == 1", "s", false},
		{"has(state.`kai.nerve`)", "s", false},
	} {
		_, err := p.Eval(c.expr, c.self, 4)
		if (err == nil) != c.valid || err != nil && strings.Contains(err.Error(), c.expr) {
			t.Errorf("%s: %v", c.expr, err)
		}
	}
}
