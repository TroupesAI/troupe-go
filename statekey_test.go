package troupe_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/TroupesAI/troupe-go"
)

// A session's state keys follow its cast and what it adds: a "cast" variable
// has a key for each character the session's cast holds, players and joiners
// included, and the session's worlds and characters bring their variables.
// Played on its own, the scenario's "cast" variables also have the keys of its
// seats marked player, which its openings may set.
func TestStateKeysSession(t *testing.T) {
	story := `{"troupe":"0.15","kind":"story",
		"characters":[{"id":"k","name":"Kai","description":"Kai."},{"id":"a","name":"Ann","description":"Ann."}],
		"scenarios":[{"id":"s","cast":[{},{"character":"k","player":true},{"character":"a"}],
			"variables":[{"id":"trust","type":"int","per":"cast","description":"T."},
				{"id":"side","type":"bool","per":["watch","guild"],"description":"S."}]}]}`
	play, _ := readPlay(t, story)
	want := []string{"scene.location", "scene.present", "state.trust.k trust s k", "state.trust.a trust s a", "state.side.watch side s watch", "state.side.guild side s guild"}
	if got := keys(play); !slices.Equal(got, want) {
		t.Errorf("on its own, the state keys are %q, want %q", got, want)
	}

	play, _ = readPlay(t, story,
		`{"troupe":"0.15","kind":"session","scenario":"s",`+
			`"characters":[{"id":"p","name":"Pat","description":"Pat.","variables":[{"id":"luck","type":"int","description":"L."}]},`+
			`{"id":"j","name":"Jo","description":"Jo."}],`+
			`"players":["p","k"],"worlds":[{"id":"h","name":"H","variables":[{"id":"rain","type":"bool","description":"R."}]}]}`)
	want = []string{
		"scene.location", "scene.present",
		"state.trust.p trust s p", "state.trust.k trust s k", "state.trust.a trust s a", "state.trust.j trust s j",
		"state.side.watch side s watch", "state.side.guild side s guild",
		"state.h.rain rain h", "state.p.luck luck p",
	}
	if got := keys(play); !slices.Equal(got, want) {
		t.Errorf("in the session, the state keys are %q, want %q", got, want)
	}
}

// A draft's cast or list that holds a key twice gives it once.
func TestStateKeysDraft(t *testing.T) {
	st, _, err := troupe.ReadStory(strings.NewReader(`{"troupe":"0.15","kind":"story",
		"characters":[{"id":"k","name":"Kai","description":"Kai."}],
		"scenarios":[{"id":"s","cast":[{"character":"k"},{"character":"k"}],
			"variables":[{"id":"trust","type":"int","per":"cast","description":"T."},
				{"id":"side","type":"bool","per":["watch","watch"],"description":"S."}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	play, err := troupe.NewScenarioPlay(st, "s")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"scene.location", "scene.present", "state.trust.k trust s k", "state.side.watch side s watch"}
	if got := keys(play); !slices.Equal(got, want) {
		t.Errorf("the state keys are %q, want %q", got, want)
	}
}

// A state gives each state key it holds as StateKeys does, with its variable,
// its owner and its key, and a "*" map's keys, which StateKeys can't list, in
// code point order.
func TestStateAll(t *testing.T) {
	play, s := readPlay(t, `{"troupe":"0.15","kind":"story",
		"characters":[{"id":"a","name":"Ann","description":"Ann.","variables":[{"id":"loot","type":"int","per":"*","description":"L."}]}],
		"scenarios":[{"id":"s","cast":[{},{"character":"a"}],
			"variables":[{"id":"bell","type":"int","description":"B."},{"id":"trust","type":"int","per":"cast","description":"T."}]}]}`,
		`{"troupe":"0.15","kind":"session","scenario":"s"}`,
		// Three keys, set in reverse order: Go ranges over a small map from a
		// random key around, which gives two keys in order half the time, but
		// never the reverse of three.
		`{"id":"1","from":"player","content":[{"text":"x"}],"changes":[{"by":"player","set":{"state.a.loot.gem":2,"state.a.loot.coin":1,"state.a.loot.axe":3}}]}`)
	var got []string
	for k := range play.Apply(play.Initial(), &s.Messages[0]).All() {
		got = append(got, describe(k))
	}
	want := []string{
		"scene.location", "scene.present", "state.bell bell s", "state.trust.a trust s a",
		"state.a.loot.axe loot a axe", "state.a.loot.coin loot a coin", "state.a.loot.gem loot a gem",
	}
	if !slices.Equal(got, want) {
		t.Errorf("the state holds %q, want %q", got, want)
	}
}

// keys returns the play's state keys, as describe gives them.
func keys(play *troupe.Play) []string {
	var out []string
	for _, k := range play.StateKeys() {
		out = append(out, describe(k))
	}
	return out
}

// describe returns a state key's name with its variable's ID, its owner and
// its key, when it has them.
func describe(k troupe.StateKey) string {
	s := k.Name
	if k.Variable != nil {
		s += " " + k.Variable.ID + " " + k.Owner
	}
	if k.Key != "" {
		s += " " + k.Key
	}
	return s
}
