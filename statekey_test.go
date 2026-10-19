package troupe_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/TroupesAI/troupe-go"
)

// A session's keys follow its cast and what it adds: a "cast" variable has a
// key for each character the session's cast holds, players and joiners
// included, and the session's worlds and characters bring their variables.
func TestStateKeysSession(t *testing.T) {
	story, _ := readStory(t, `{"troupe":"0.14","kind":"bundle",
		"characters":[{"id":"k","name":"Kai","description":"Kai."},{"id":"a","name":"Ann","description":"Ann."}],
		"scenarios":[{"id":"s","cast":[{},{"character":"k","player":true},{"character":"a"}],
			"variables":[{"id":"trust","type":"int","of":"cast","description":"T."},
				{"id":"side","type":"bool","of":["watch","guild"],"description":"S."}]}]}`,
		`{"troupe":"0.14","kind":"session","scenario":"s",`+
			`"characters":[{"id":"p","name":"Pat","description":"Pat.","variables":[{"id":"luck","type":"int","description":"L."}]},`+
			`{"id":"j","name":"Jo","description":"Jo."}],`+
			`"players":["p","k"],"worlds":[{"id":"h","name":"H","variables":[{"id":"rain","type":"bool","description":"R."}]}]}`)
	want := []string{
		"scene.location", "scene.present",
		"state.trust.p trust s", "state.trust.k trust s", "state.trust.a trust s", "state.trust.j trust s",
		"state.side.watch side s", "state.side.guild side s",
		"state.h.rain rain h", "state.p.luck luck p",
	}
	if got := keys(story); !slices.Equal(got, want) {
		t.Errorf("the keys are %q, want %q", got, want)
	}
}

// A draft's cast or list that holds a key twice gives it once.
func TestStateKeysDraft(t *testing.T) {
	b, _, err := troupe.ReadBundle(strings.NewReader(`{"troupe":"0.14","kind":"bundle",
		"characters":[{"id":"k","name":"Kai","description":"Kai."}],
		"scenarios":[{"id":"s","cast":[{"character":"k"},{"character":"k"}],
			"variables":[{"id":"trust","type":"int","of":"cast","description":"T."},
				{"id":"side","type":"bool","of":["watch","watch"],"description":"S."}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	story, err := troupe.NewScenarioStory(b, "s")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"scene.location", "scene.present", "state.trust.k trust s", "state.side.watch side s"}
	if got := keys(story); !slices.Equal(got, want) {
		t.Errorf("the keys are %q, want %q", got, want)
	}
}

// keys returns the story's keys, each with its variable's ID and owner.
func keys(story *troupe.Story) []string {
	var out []string
	for _, k := range story.StateKeys() {
		s := k.Name
		if k.Variable != nil {
			s += " " + k.Variable.ID + " " + k.Owner
		}
		out = append(out, s)
	}
	return out
}
