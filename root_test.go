package troupe_test

import (
	"encoding/json/v2"
	"maps"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/TroupesAI/troupe-go"
)

// A root made from an opening holds its content with each pick chosen and
// escaped, so the root shows exactly the option, and records its set by
// "opening", with the players added to the scene.present it sets. Written as
// a session's root, it conforms; an opening without content or set makes a
// root with neither.
func TestRoot(t *testing.T) {
	st, diags, err := troupe.ReadStory(strings.NewReader(`{"troupe":"0.15","kind":"story",
		"characters":[{"id":"a","name":"Ann","description":"Ann."}],
		"scenarios":[{"id":"s","cast":[{},{"character":"a"}],
			"variables":[{"id":"bell","type":"int","description":"d"}],
			"openings":[
				{"id":"o","set":{"state.bell":7,"scene.present":["a"]},"content":[
					{"speaker":"a","text":"{{char}} says {{pick::\\::{{x}}."},
					{"type":"image","src":"https://x.test/a.png","alt":"{{pick::one::two}}"}]},
				{"id":"bare"}]}]}`))
	if err != nil || diags != nil {
		t.Fatal(err, diags)
	}
	header := `{"troupe":"0.15","kind":"session","scenario":"s","characters":[{"id":"p","name":"Pat","description":"Pat."}],"players":["p"]}`
	s, diags, err := troupe.ReadSession(strings.NewReader(header))
	if err != nil || diags != nil {
		t.Fatal(err, diags)
	}
	play, err := troupe.NewPlay(st, s)
	if err != nil {
		t.Fatal(err)
	}
	r := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // G404: a seeded source makes the test reproducible

	shown := map[string]bool{}
	for range 20 {
		root, err := play.Root("o", r)
		if err != nil {
			t.Fatal(err)
		}
		text := root.Content[0].Text
		if text != `{{char}} says \\.` && text != `{{char}} says \{{x.` {
			t.Fatalf("the root's text is %q", text)
		}
		parts := play.RenderParts(play.Initial(), &root, r)
		shown[parts[0].Text] = true
		if alt := root.Content[1].Alt; alt != "one" && alt != "two" {
			t.Errorf("the root's alt is %q", alt)
		}
		if len(root.Changes) != 1 || root.Changes[0].By != "opening" ||
			string(root.Changes[0].Set["state.bell"]) != "7" || string(root.Changes[0].Set["scene.present"]) != `["a","p"]` {
			t.Fatalf("the root's changes are %v", root.Changes)
		}
	}
	if !slices.Equal(slices.Sorted(maps.Keys(shown)), []string{`Ann says \.`, `Ann says {{x.`}) {
		t.Errorf("the roots show %v", shown)
	}

	bare, err := play.Root("bare", r)
	if err != nil {
		t.Fatal(err)
	}
	if bare.Content == nil || len(bare.Content) != 0 || bare.Changes != nil {
		t.Errorf("a root from an opening without content or set is %+v", bare)
	}

	lines := []string{header}
	for i, id := range []string{"o", "bare"} {
		root, _ := play.Root(id, r)
		root.ID = string(rune('1' + i))
		line, err := json.Marshal(root)
		if err != nil {
			t.Fatal(err)
		}
		lines = append(lines, string(line))
	}
	s, diags, err = troupe.ReadSession(strings.NewReader(strings.Join(lines, "\n")))
	if err != nil || diags != nil {
		t.Fatal(err, diags)
	}
	if diags := troupe.Check(st, s); diags != nil {
		t.Errorf("the roots, written, are %v", diags)
	}

	if _, err := play.Root("missing", r); err == nil {
		t.Error("a root from an opening the scenario doesn't have")
	}
}

// A root leaves out the state keys of a seat marked player that no player
// fills: its character's variables and its key of "cast" variables, whoever
// owns them. A seat a player fills keeps them, and a set left without state
// keys makes no change.
func TestRootSeatKeys(t *testing.T) {
	story := `{"troupe":"0.15","kind":"story",
		"characters":[{"id":"k","name":"Kai","description":"Kai.","variables":[{"id":"nerve","type":"int","description":"N."}]},
			{"id":"a","name":"Ann","description":"Ann.","variables":[{"id":"likes","type":"bool","per":"cast","description":"L."}]}],
		"scenarios":[{"id":"s","cast":[{"character":"k","player":true},{"character":"a"}],
			"variables":[{"id":"trust","type":"int","per":"cast","description":"T."}],
			"openings":[{"id":"o","set":{"state.k.nerve":1,"state.trust.k":2,"state.trust.a":3,"state.a.likes.k":true}},
				{"id":"seat","set":{"state.k.nerve":1}}]}]}`
	for _, tc := range []struct {
		players string
		keys    []string
	}{
		{`["p"]`, []string{"state.trust.a"}},
		{`["p","k"]`, []string{"state.a.likes.k", "state.k.nerve", "state.trust.a", "state.trust.k"}},
	} {
		header := `{"troupe":"0.15","kind":"session","scenario":"s","characters":[{"id":"p","name":"Pat","description":"Pat."}],"players":` + tc.players + `}`
		play, _ := readPlay(t, story, header)
		root, err := play.Root("o", nil)
		if err != nil {
			t.Fatal(err)
		}
		if got := slices.Sorted(maps.Keys(root.Changes[0].Set)); !slices.Equal(got, tc.keys) {
			t.Errorf("players %s: the root sets %v, want %v", tc.players, got, tc.keys)
		}
		root.ID = "1"
		line, err := json.Marshal(root)
		if err != nil {
			t.Fatal(err)
		}
		readPlay(t, story, header, string(line))
	}

	play, _ := readPlay(t, story, `{"troupe":"0.15","kind":"session","scenario":"s","characters":[{"id":"p","name":"Pat","description":"Pat."}],"players":["p"]}`)
	if root, _ := play.Root("seat", nil); root.Changes != nil {
		t.Errorf("a root whose set has only a seat's keys records %v", root.Changes)
	}
}
