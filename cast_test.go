package troupe_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/TroupesAI/troupe-go"
)

const castBundle = `{"troupe":"0.14","kind":"bundle",
	"characters":[
		{"id":"a","name":"Ann","description":"Ann."},
		{"id":"b","name":"Bo","description":"Bo."},
		{"id":"k","name":"Kai","description":"Kai."}],
	"worlds":[{"id":"w","name":"W"},{"id":"v","name":"V"}],
	"scenarios":[
		{"id":"s","mode":"narrator","narrator":"the Chorus","worlds":["v"],"cast":[
			{"notes":"N"},{"character":"a","notes":"N","spoiler":true},{"character":"k","player":true},{"character":"b"},{}]},
		{"id":"t"}]}`

// A session's cast fills the seats in order, follows them with new seats and
// the joiners, and marks whom people play. A scenario played on its own lists
// its cast as written, seats included, and applies the defaults.
func TestCast(t *testing.T) {
	story, _ := readStory(t, castBundle, `{"troupe":"0.14","kind":"session","scenario":"s",`+
		`"characters":[{"id":"p","name":"Pat","description":"Pat."},{"id":"q","name":"Quin","description":"Quin."},`+
		`{"id":"r","name":"Rue","description":"Rue."},{"id":"j","name":"Jo","description":"Jo."}],`+
		`"players":["p","k","q","b","r"],"worlds":[{"id":"h","name":"H"}]}`)
	check(t, "the session", story,
		[]string{"p seat player N", "a N spoiler", "k seat player", "b player", "q seat player", "r seat player", "j"},
		[]string{"v", "h"}, "narrator", "the Chorus")

	b, _, err := troupe.ReadBundle(strings.NewReader(castBundle))
	if err != nil {
		t.Fatal(err)
	}
	story, err = troupe.NewScenarioStory(b, "s")
	if err != nil {
		t.Fatal(err)
	}
	check(t, "s on its own", story, []string{"- seat N", "a N spoiler", "k seat", "b", "- seat"}, []string{"v"}, "narrator", "the Chorus")
	story, err = troupe.NewScenarioStory(b, "t")
	if err != nil {
		t.Fatal(err)
	}
	check(t, "t on its own", story, []string{"- seat", "a", "b", "k"}, []string{"w", "v"}, "characters", "the narrator")
	if _, err := troupe.NewScenarioStory(b, "missing"); err == nil {
		t.Error("a story of a scenario the bundle doesn't have")
	}
}

func check(t *testing.T, name string, story *troupe.Story, cast, worlds []string, mode, narrator string) {
	t.Helper()
	var got []string
	for _, m := range story.Cast() {
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
	for _, w := range story.Worlds() {
		ids = append(ids, w.ID)
	}
	if !slices.Equal(ids, worlds) {
		t.Errorf("%s: the worlds are %v, want %v", name, ids, worlds)
	}
	if got := fmt.Sprint(story.Mode(), story.Narrator()); got != fmt.Sprint(mode, narrator) {
		t.Errorf("%s: the mode and narrator are %s", name, got)
	}
}
