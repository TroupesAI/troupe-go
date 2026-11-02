package troupe_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/TroupesAI/troupe-go"
)

const castStory = `{"troupe":"0.15","kind":"story",
	"characters":[
		{"id":"a","name":"Ann","description":"Ann."},
		{"id":"b","name":"Bo","description":"Bo."},
		{"id":"k","name":"Kai","description":"Kai."}],
	"worlds":[{"id":"w","name":"W"},{"id":"v","name":"V"}],
	"scenarios":[
		{"id":"s","mode":"narrator","narratorName":"the Chorus","worlds":["v"],"cast":[
			{"notes":"N"},{"character":"a","notes":"N","spoiler":true},{"character":"k","player":true},{"character":"b"},{}]},
		{"id":"t"}]}`

// A session's cast fills the seats in order, follows them with new seats and
// the joiners, and marks whom people play. A scenario played on its own lists
// its cast as written, seats included, and applies the defaults.
func TestCast(t *testing.T) {
	play, _ := readPlay(t, castStory, `{"troupe":"0.15","kind":"session","scenario":"s",`+
		`"characters":[{"id":"p","name":"Pat","description":"Pat."},{"id":"q","name":"Quin","description":"Quin."},`+
		`{"id":"r","name":"Rue","description":"Rue."},{"id":"j","name":"Jo","description":"Jo."}],`+
		`"players":["p","k","q","b","r"],"worlds":[{"id":"h","name":"H"}]}`)
	check(t, "the session", play,
		[]string{"p seat player N", "a N spoiler", "k seat player", "b player", "q seat player", "r seat player", "j"},
		[]string{"v", "h"}, "narrator", "the Chorus")

	st, _, err := troupe.ReadStory(strings.NewReader(castStory))
	if err != nil {
		t.Fatal(err)
	}
	play, err = troupe.NewScenarioPlay(st, "s")
	if err != nil {
		t.Fatal(err)
	}
	check(t, "s on its own", play, []string{"- seat N", "a N spoiler", "k seat", "b", "- seat"}, []string{"v"}, "narrator", "the Chorus")
	play, err = troupe.NewScenarioPlay(st, "t")
	if err != nil {
		t.Fatal(err)
	}
	check(t, "t on its own", play, []string{"- seat", "a", "b", "k"}, []string{"w", "v"}, "characters", "the narrator")
	if _, err := troupe.NewScenarioPlay(st, "missing"); err == nil {
		t.Error("a play of a scenario the story doesn't have")
	}
}

func check(t *testing.T, name string, play *troupe.Play, cast, worlds []string, mode, narratorName string) {
	t.Helper()
	var got []string
	for _, m := range play.Cast() {
		s := "-"
		if m.Character != nil {
			s = m.Character.ID
		}
		for _, f := range []struct {
			on   bool
			word string
		}{{m.Seat, "seat"}, {m.Player, "player"}, {m.Notes != "", m.Notes}, {m.Spoiler, "spoiler"}} {
			if f.on {
				s += " " + f.word
			}
		}
		got = append(got, s)
	}
	if !slices.Equal(got, cast) {
		t.Errorf("%s: the cast is %q, want %q", name, got, cast)
	}
	var ids []string
	for _, w := range play.Worlds() {
		ids = append(ids, w.ID)
	}
	if !slices.Equal(ids, worlds) {
		t.Errorf("%s: the worlds are %v, want %v", name, ids, worlds)
	}
	if got := fmt.Sprint(play.Mode(), play.NarratorName()); got != fmt.Sprint(mode, narratorName) {
		t.Errorf("%s: the mode and narrator are %s", name, got)
	}
}

// A spoiler stays one on a path until a state at a message on it has the
// character present, then stays revealed after it leaves, but not on another
// branch. A stored state keeps it.
func TestSpoilers(t *testing.T) {
	story := `{"troupe":"0.15","kind":"story",
		"characters":[{"id":"a","name":"Ann","description":"Ann."},{"id":"g","name":"Ghost","description":"A ghost."}],
		"scenarios":[{"id":"s","cast":[{},{"character":"a"},{"character":"g","spoiler":true}],
			"openings":[{"id":"o","set":{"scene.present":["a"]}}]}]}`
	play, s := readPlay(t, story,
		`{"troupe":"0.15","kind":"session","scenario":"s","characters":[{"id":"p","name":"Pat","description":"Pat."}],"players":["p"]}`,
		`{"id":"1","from":"system","opening":"o","content":[],"changes":[{"by":"opening","set":{"scene.present":["a","p"]}}]}`,
		`{"id":"2","parent":"1","from":"model","char":"a","content":[{"text":"x"}],"changes":[{"by":"model","set":{"scene.present":["g"]}}]}`,
		`{"id":"3","parent":"2","from":"model","char":"a","content":[{"text":"x"}],"changes":[{"by":"model","set":{"scene.present":["a"]}}]}`,
		`{"id":"4","parent":"1","from":"model","char":"a","content":[{"text":"x"}]}`)
	want := map[string][]string{"1": {"g"}, "2": nil, "3": nil, "4": {"g"}}
	states := map[string]troupe.State{"": play.Initial()}
	for _, m := range s.Messages {
		states[m.ID] = play.Apply(states[m.Parent], &m)
		data, err := states[m.ID].MarshalJSON()
		if err != nil {
			t.Fatal(err)
		}
		restored, err := play.UnmarshalState(data)
		if err != nil {
			t.Fatal(err)
		}
		for name, state := range map[string]troupe.State{"applied": states[m.ID], "restored": restored} {
			if got := state.Spoilers(); !slices.Equal(got, want[m.ID]) {
				t.Errorf("at message %s, %s, the spoilers are %v, want %v", m.ID, name, got, want[m.ID])
			}
		}
	}
}
