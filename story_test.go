package troupe_test

import (
	"maps"
	"reflect"
	"strings"
	"testing"

	"github.com/TroupesAI/troupe-go"
)

// Applying each message to its parent's state gives the state StateAt gives,
// so a caller that keeps states never replays a path. States are immutable:
// applying a message leaves its parent's state as it was, and what Get and All
// return can't change a state. Get reads what All yields, which is the scene
// and the variables: the player {{user}} means is kept in a state, but isn't
// one of its keys.
func TestStoryApply(t *testing.T) {
	b, diags, err := troupe.ReadBundle(strings.NewReader(`{"troupe":"0.14","kind":"bundle",
		"characters":[{"id":"a","name":"Ann","description":"Ann."}],
		"scenarios":[{"id":"s","cast":[{},{"character":"a"}],"variables":[
			{"id":"bell","type":"int","min":0,"max":12,"description":"d"},
			{"id":"trust","type":"int","of":"cast","initial":{"*":50},"description":"d"},
			{"id":"inv","type":"int","of":"*","description":"d"},
			{"id":"tags","type":"set","values":["a","b"],"description":"d"}]}]}`))
	if err != nil || diags != nil {
		t.Fatal(err, diags)
	}
	// Messages 2 and 3 are siblings, so each starts from 1's state.
	s, diags, err := troupe.ReadSession(strings.NewReader(strings.Join([]string{
		`{"troupe":"0.14","kind":"session","scenario":"s","characters":[{"id":"p","name":"Pat","description":"Pat."}],"players":["p"]}`,
		`{"id":"1","from":"player","content":[{"text":"x"}],"changes":[{"by":"player","set":{"state.bell":20,"state.inv.torch":2,"state.tags":["a"]}}]}`,
		`{"id":"2","parent":"1","from":"player","content":[{"text":"x"}],"changes":[{"by":"player","set":{"state.trust.a":9,"state.tags":["b","a","b"]}},{"by":"player","set":{"state.inv.torch":0,"scene.present":["a"]}}]}`,
		`{"id":"3","parent":"1","from":"player","content":[{"text":"x"}],"changes":[{"by":"player","set":{"scene.location":""}}]}`,
	}, "\n")))
	if err != nil || diags != nil {
		t.Fatal(err, diags)
	}
	story, err := troupe.NewStory(b, s)
	if err != nil {
		t.Fatal(err)
	}
	// StateAt runs on a story of its own, so a state Apply changes in place
	// can't change what it is compared with.
	fresh, err := troupe.NewStory(b, s)
	if err != nil {
		t.Fatal(err)
	}
	states := map[string]troupe.State{}
	for _, m := range s.Messages {
		before := story.Initial()
		if m.Parent != "" {
			before = states[m.Parent]
		}
		states[m.ID] = story.Apply(before, &m)
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
		if x, ok := states[m.ID].Get("user"); ok {
			t.Errorf("message %s: Get(\"user\") is %v", m.ID, x)
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
