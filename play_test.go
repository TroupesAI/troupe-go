package troupe_test

import (
	"fmt"
	"maps"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/TroupesAI/troupe-go"
)

// Applying each message to its parent's state gives the state StateAt gives,
// so a caller that keeps states never replays a path. States are immutable:
// applying a message leaves its parent's state as it was, and what Get and All
// return can't change a state. Get reads what All yields, which is the scene
// and the variables: the player {{player}} means is kept in a state, but isn't
// one of its state keys.
func TestPlayApply(t *testing.T) {
	st, diags, err := troupe.ReadStory(strings.NewReader(`{"troupe":"0.15","kind":"story",
		"characters":[{"id":"a","name":"Ann","description":"Ann."}],
		"scenarios":[{"id":"s","cast":[{},{"character":"a"}],"variables":[
			{"id":"bell","type":"int","min":0,"max":12,"description":"d"},
			{"id":"trust","type":"int","per":"cast","initial":{"*":50},"description":"d"},
			{"id":"inv","type":"int","per":"*","description":"d"},
			{"id":"tags","type":"set","values":["a","b"],"description":"d"}]}]}`))
	if err != nil || diags != nil {
		t.Fatal(err, diags)
	}
	// Messages 2 and 3 are siblings, so each starts from 1's state.
	s, diags, err := troupe.ReadSession(strings.NewReader(strings.Join([]string{
		`{"troupe":"0.15","kind":"session","scenario":"s","characters":[{"id":"p","name":"Pat","description":"Pat."}],"players":["p"]}`,
		`{"id":"1","from":"player","content":[{"text":"x"}],"changes":[{"by":"player","set":{"state.bell":20,"state.inv.torch":2,"state.tags":["a"]}}]}`,
		`{"id":"2","parent":"1","from":"player","content":[{"text":"x"}],"changes":[{"by":"player","set":{"state.trust.a":9,"state.tags":["b","a","b"]}},{"by":"player","set":{"state.inv.torch":0,"scene.present":["a"]}}]}`,
		`{"id":"3","parent":"1","from":"player","content":[{"text":"x"}],"changes":[{"by":"player","set":{"scene.location":""}}]}`,
	}, "\n")))
	if err != nil || diags != nil {
		t.Fatal(err, diags)
	}
	play, err := troupe.NewPlay(st, s)
	if err != nil {
		t.Fatal(err)
	}
	// StateAt runs on a play of its own, so a state Apply changes in place
	// can't change what it is compared with.
	fresh, err := troupe.NewPlay(st, s)
	if err != nil {
		t.Fatal(err)
	}
	states := map[string]troupe.State{}
	for _, m := range s.Messages {
		before := play.Initial()
		if m.Parent != "" {
			before = states[m.Parent]
		}
		states[m.ID] = play.Apply(before, &m)
	}
	for _, m := range s.Messages {
		want, err := fresh.StateAt(m.ID)
		if err != nil {
			t.Fatal(err)
		}
		got := maps.Collect(states[m.ID].All())
		if !reflect.DeepEqual(got, maps.Collect(want.All())) {
			t.Errorf("message %s: Apply gives %v, StateAt %v", m.ID, got, maps.Collect(want.All()))
		}
		for key, x := range got {
			if key != "scene.location" && key != "scene.present" && !strings.HasPrefix(key, "state.") {
				t.Errorf("message %s: All yields %q", m.ID, key)
			}
			if y, ok := states[m.ID].Get(key); !ok || !reflect.DeepEqual(x, y) {
				t.Errorf("message %s: Get(%q) is %v, %v; All yields %v", m.ID, key, y, ok, x)
			}
		}
		if x, ok := states[m.ID].Get("player"); ok {
			t.Errorf("message %s: Get(\"player\") is %v", m.ID, x)
		}
	}

	tags, _ := states["1"].Get("state.tags")
	tags.([]string)[0] = "changed"
	for _, x := range states["1"].All() {
		if set, ok := x.([]string); ok && len(set) > 0 {
			set[0] = "changed"
		}
	}
	if got, _ := states["1"].Get("state.tags"); !reflect.DeepEqual(got, []string{"a"}) {
		t.Errorf("state.tags is %v after changing what Get and All return", got)
	}
}

// Within gives scene.within as locations: the scene's location, then each
// parent, in whichever world of the story or the session it is, in use or not.
// A condition at the state reads the same chain, and an empty one before the
// scene has a location. The chain holds every location, the longest there can
// be, so the count that ends a cycle's walk must let that many through.
func TestWithin(t *testing.T) {
	play, s := readPlay(t, `{"troupe":"0.15","kind":"story",
		"worlds":[
			{"id":"city","name":"City","locations":[{"id":"docks","name":"The docks"},{"id":"pier","name":"Pier 9","parent":"docks"}]},
			{"id":"sea","name":"Sea","locations":[{"id":"ship","name":"The Gull","parent":"pier"}]}],
		"scenarios":[{"id":"s","worlds":["city"]}]}`,
		`{"troupe":"0.15","kind":"session","scenario":"s","worlds":[{"id":"below","name":"Below","locations":[{"id":"hold","name":"The hold","parent":"ship"}]}]}`,
		`{"id":"1","from":"player","content":[{"text":"x"}],"changes":[{"by":"player","set":{"scene.location":"hold"}}]}`)
	for _, c := range []struct {
		state       troupe.State
		names, expr string
	}{
		{play.Initial(), "", "scene.within == []"},
		{play.Apply(play.Initial(), &s.Messages[0]), "The hold, The Gull, Pier 9, The docks", "scene.within == ['hold', 'ship', 'pier', 'docks']"},
	} {
		var names []string
		for _, l := range c.state.Within() {
			names = append(names, l.Name)
		}
		if got := strings.Join(names, ", "); got != c.names {
			t.Errorf("the scene is within %q, want %q", got, c.names)
		}
		if ok, err := play.PointAt(c.state, nil, "").Eval(c.expr, "s", 4); err != nil || !ok {
			t.Errorf("%s is %v, %v", c.expr, ok, err)
		}
	}
}

// One read story serves plays of different sessions at once: the package
// only reads it.
func TestSharedStory(t *testing.T) {
	st, diags, err := troupe.ReadStory(strings.NewReader(`{"troupe":"0.15","kind":"story",
		"worlds":[{"id":"w1","name":"W","description":"d"},{"id":"w2","name":"W","description":"d"},{"id":"w3","name":"W","description":"d"}],
		"scenarios":[{"id":"s","worlds":["w1","w2","w3"]}]}`))
	if err != nil || diags != nil {
		t.Fatal(err, diags)
	}
	var wg sync.WaitGroup
	for i := range 4 {
		id := fmt.Sprintf("x%d", i)
		s, diags, err := troupe.ReadSession(strings.NewReader(`{"troupe":"0.15","kind":"session","scenario":"s","worlds":[{"id":"` + id + `","name":"X","description":"d"}]}`))
		if err != nil || diags != nil {
			t.Fatal(err, diags)
		}
		wg.Go(func() {
			for range 100 {
				play, err := troupe.NewPlay(st, s)
				if err != nil {
					t.Error(err)
					return
				}
				if w := play.Worlds(); len(w) != 4 || w[3].ID != id {
					t.Errorf("session %s: %d worlds", id, len(w))
					return
				}
			}
		})
	}
	wg.Wait()
}
