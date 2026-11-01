package troupe_test

import (
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/TroupesAI/troupe-go"
)

// activationStory has entries for each rule of activation (Format 7.2).
const activationStory = `{"troupe":"0.15","kind":"story",
	"characters":[
		{"id":"a","name":"Ann","description":"Ann.","entries":[{"id":"own","text":"t"}]},
		{"id":"b","name":"Bo","description":"Bo.","entries":[{"id":"own","text":"t"}]}],
	"worlds":[{"id":"w","name":"W","entries":[{"id":"lore","text":"t","keywords":["/drag(on|ons)/i"],"scanDepth":2}]}],
	"scenarios":[{"id":"s","cast":[{},{"character":"a"},{"character":"b"}],
		"variables":[
			{"id":"n","type":"int","by":["player"],"description":"d"},
			{"id":"note","type":"string","by":["model"],"description":"d"},
			{"id":"trust","type":"int","per":"cast","initial":5,"description":"d"}],
		"entries":[
			{"id":"always","text":"t"},
			{"id":"keyed","text":"t","keywords":["rope"],"scanDepth":1},
			{"id":"cond","text":"t","when":"state.n >= 2"},
			{"id":"retrieved","text":"t","title":"T","retrievable":true},
			{"id":"off","text":"t","disabled":true},
			{"id":"secret","text":"t","knownBy":["b"]},
			{"id":"once","text":"t","once":true},
			{"id":"sticky","text":"t","keywords":["bell"],"scanDepth":1,"sticky":1},
			{"id":"cool","text":"t","cooldown":1},
			{"id":"never","text":"t","chance":0},
			{"id":"low","text":"t","group":"g","priority":10},
			{"id":"high","text":"t","group":"g","priority":90},
			{"id":"chosen","text":"t","group":"h","priority":90,"chance":0},
			{"id":"passed","text":"t","group":"h"}]}]}`

// activationSession's path is the rounds 1 to 3: message 2 records entries
// as activated, 3 adds a session entry and 4 removes it.
var activationSession = []string{
	`{"troupe":"0.15","kind":"session","scenario":"s","characters":[{"id":"p","name":"Pat","description":"Pat."}],"players":["p"]}`,
	`{"id":"1","from":"player","content":[{"speaker":"p","text":"A rope, a bell, Dragons."}]}`,
	`{"id":"2","parent":"1","from":"model","char":"a","content":[{"speaker":"a","text":"Quiet."}],"activated":["s/once","s/sticky","s/cool"]}`,
	`{"id":"3","parent":"2","from":"player","content":[{"text":"Quiet."}],"changes":[{"by":"player","set":{"state.n":2,"scene.present":["a"]}}],"entries":[{"id":"mem","text":"t","knownBy":["a"]}]}`,
	`{"id":"4","parent":"3","from":"player","content":[{"text":"Quiet."}],"entries":[{"id":"mem","text":"t","disabled":true}]}`,
}

// Entries activate by their keywords, when and timing, for whoever the message
// being written is for, with one per group, from the entries in use.
func TestActive(t *testing.T) {
	play, s := readPlay(t, activationStory, activationSession...)
	r := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // G404: a seeded source makes the test reproducible
	for _, tc := range []struct {
		at, char string
		want     []string
	}{
		{"1", "a", []string{"s/always", "s/keyed", "s/once", "s/sticky", "s/cool", "s/high", "w/lore", "a/own", "b/own"}},
		{"1", "b", []string{"s/always", "s/keyed", "s/secret", "s/once", "s/sticky", "s/cool", "s/high", "w/lore", "a/own", "b/own"}},
		// Message 2 is for a, in round 1: sticky keeps its entry, and cooldown
		// blocks its own for the rest of the round.
		{"2", "a", []string{"s/always", "s/sticky kept", "s/high", "w/lore", "a/own", "b/own"}},
		// In round 2, b has left, and a session entry only a knows is added.
		{"3", "a", []string{"s/always", "s/cond", "s/sticky kept", "s/high", "a/own", "session/mem"}},
		{"3", "", []string{"s/always", "s/cond", "s/sticky kept", "s/high", "a/own"}},
		{"3", "b", []string{"s/always", "s/cond", "s/secret", "s/sticky kept", "s/high", "a/own", "b/own"}},
		// In round 3, sticky and cooldown are over, and the session entry is
		// removed.
		{"4", "a", []string{"s/always", "s/cond", "s/cool", "s/high", "a/own"}},
	} {
		path, err := pathTo(s, tc.at)
		if err != nil {
			t.Fatal(err)
		}
		got := refs(play.PointAt(play.Initial(), path, tc.char).Active(nil, r))
		if !slices.Equal(got, tc.want) {
			t.Errorf("at %s for %q: %v, want %v", tc.at, tc.char, got, tc.want)
		}
	}
}

// chance lets an entry through as often as it says, and among a group's
// entries of equal priority, weight chooses as often as it says.
func TestActiveChanceAndWeight(t *testing.T) {
	play, _ := readPlay(t, `{"troupe":"0.15","kind":"story","scenarios":[{"id":"s","entries":[
		{"id":"half","text":"t","chance":50},
		{"id":"light","text":"t","group":"g"},
		{"id":"heavy","text":"t","group":"g","weight":3}]}]}`)
	r := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // G404: a seeded source makes the test reproducible
	p := play.PointAt(play.Initial(), nil, "")
	count := map[string]int{}
	const n = 4000
	for range n {
		for _, ref := range refs(p.Active(nil, r)) {
			count[ref]++
		}
	}
	if count["s/light"]+count["s/heavy"] != n {
		t.Errorf("a group had other than one entry active: %v", count)
	}
	if half := count["s/half"]; half < n*45/100 || half > n*55/100 {
		t.Errorf("chance 50 let an entry through %d times in %d", half, n)
	}
	if light := count["s/light"]; light < n*20/100 || light > n*30/100 {
		t.Errorf("weight 1 against 3 chose an entry %d times in %d", light, n)
	}
}

// An entry sticky keeps wins its group against equal priorities, and passes
// its chance, with no draw, and gives way to a higher priority (Format 7.2).
func TestActiveStickyInGroup(t *testing.T) {
	play, s := readPlay(t, `{"troupe":"0.15","kind":"story","scenarios":[{"id":"s","entries":[
		{"id":"kept","text":"t","group":"g","sticky":2,"chance":0},
		{"id":"heavy","text":"t","group":"g","weight":1000},
		{"id":"low","text":"t","group":"h","sticky":2},
		{"id":"high","text":"t","group":"h","priority":90}]}]}`,
		`{"troupe":"0.15","kind":"session","scenario":"s","characters":[{"id":"p","name":"Pat","description":"Pat."}],"players":["p"]}`,
		`{"id":"1","from":"player","content":[{"speaker":"p","text":"Hi."}]}`,
		`{"id":"2","parent":"1","from":"model","content":[{"text":"Hi."}],"activated":["s/kept","s/low"]}`)
	r := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // G404: a seeded source makes the test reproducible
	p := play.PointAt(play.Initial(), s.Messages, "")
	for range 100 {
		if got := refs(p.Active(nil, r)); !slices.Equal(got, []string{"s/kept kept", "s/high"}) {
			t.Fatalf("%v, want s/kept kept and s/high", got)
		}
	}
}

// A retrieved entry activates under every rule of activation, so it can push
// a keyed entry of lower priority out of its group, and only an entry that
// retrieving would activate and that can win its group is offered (Format
// 7.2).
func TestActiveRetrieved(t *testing.T) {
	play, s := readPlay(t, `{"troupe":"0.15","kind":"story","scenarios":[{"id":"s","entries":[
		{"id":"keyed","text":"t","keywords":["rope"],"group":"g","priority":40},
		{"id":"retrieved","text":"t","title":"T","retrievable":true,"group":"g","priority":60},
		{"id":"equal","text":"t","title":"T","retrievable":true,"group":"g","priority":40},
		{"id":"weak","text":"t","title":"T","retrievable":true,"group":"g","priority":30},
		{"id":"unmatched","text":"t","keywords":["bell"]},
		{"id":"both","text":"t","title":"T","keywords":["rope"],"retrievable":true},
		{"id":"barred","text":"t","title":"T","retrievable":true,"when":"false"}]}]}`,
		activationSession[0], `{"id":"1","from":"player","content":[{"speaker":"p","text":"A rope."}]}`)
	r := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // G404: a seeded source makes the test reproducible
	p := play.PointAt(play.Initial(), s.Messages, "")
	if got := refs(p.Retrievable()); !slices.Equal(got, []string{"s/retrieved", "s/equal"}) {
		t.Errorf("retrievable: %v, want s/retrieved and s/equal", got)
	}
	if got := refs(p.Active(nil, r)); !slices.Equal(got, []string{"s/keyed", "s/both"}) {
		t.Errorf("without retrieval: %v, want s/keyed and s/both", got)
	}
	if got := refs(p.Active([]string{"s/retrieved", "s/equal", "s/weak", "s/barred", "s/unmatched"}, r)); !slices.Equal(got, []string{"s/retrieved", "s/both"}) {
		t.Errorf("retrieved: %v, want s/retrieved and s/both", got)
	}
}

// readPlay reads a story and a session, given as its lines, or none, which
// conform, and returns their play.
func readPlay(t *testing.T, story string, session ...string) (*troupe.Play, *troupe.Session) {
	t.Helper()
	st, diags, err := troupe.ReadStory(strings.NewReader(story))
	if err != nil || diags != nil {
		t.Fatal(err, diags)
	}
	var s *troupe.Session
	if session != nil {
		s, diags, err = troupe.ReadSession(strings.NewReader(strings.Join(session, "\n")))
		if err != nil || diags != nil {
			t.Fatal(err, diags)
		}
	}
	if diags := troupe.Check(st, s); diags != nil {
		t.Fatal(diags)
	}
	play, err := troupe.NewPlay(st, s)
	if err != nil {
		t.Fatal(err)
	}
	return play, s
}

// refs returns the references of active entries, with " kept" after those
// sticky keeps active.
func refs(active []troupe.ActiveEntry) []string {
	var out []string
	for _, a := range active {
		ref := a.Owner + "/" + a.Entry.ID
		if a.Kept {
			ref += " kept"
		}
		out = append(out, ref)
	}
	return out
}
