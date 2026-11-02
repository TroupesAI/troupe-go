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
	want := []string{"scene.location", "scene.present", "state.trust.k trust s", "state.trust.a trust s", "state.side.watch side s", "state.side.guild side s"}
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
		"state.trust.p trust s", "state.trust.k trust s", "state.trust.a trust s", "state.trust.j trust s",
		"state.side.watch side s", "state.side.guild side s",
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
	want := []string{"scene.location", "scene.present", "state.trust.k trust s", "state.side.watch side s"}
	if got := keys(play); !slices.Equal(got, want) {
		t.Errorf("the state keys are %q, want %q", got, want)
	}
}

// keys returns the play's state keys, each with its variable's ID and owner.
func keys(play *troupe.Play) []string {
	var out []string
	for _, k := range play.StateKeys() {
		s := k.Name
		if k.Variable != nil {
			s += " " + k.Variable.ID + " " + k.Owner
		}
		out = append(out, s)
	}
	return out
}
