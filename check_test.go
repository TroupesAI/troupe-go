package troupe_test

import (
	"encoding/json/v2"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/TroupesAI/troupe-go"
)

// own means the variables of the object a text or condition belongs to
// (Format 8.2), even when another owner shares its ID, so Check reports only
// the DUPLICATE_ID.
func TestCheckOwnSharedID(t *testing.T) {
	for _, c := range []struct{ name, bundle, at string }{
		{"scenario", `"characters":[{"id":"kai","name":"Kai","description":"{{own.nerve}}",
			"variables":[{"id":"nerve","type":"int","description":"n"}],
			"entries":[{"id":"e","when":"own.nerve >= 2","text":"t"}]}],
			"scenarios":[{"id":"kai","variables":[{"id":"bell","type":"int","description":"b"}],
			"entries":[{"id":"p","when":"own.bell > 0","text":"t"}]}]`, "/scenarios/0/id"},
		{"character", `"characters":[{"id":"kai","name":"Kai","description":"d"},
			{"id":"kai","name":"Kai","description":"{{own.nerve}}",
			"variables":[{"id":"nerve","type":"int","description":"n"}],
			"entries":[{"id":"e","when":"own.nerve >= 2","text":"t"}]}],
			"scenarios":[{"id":"s"}]`, "/characters/1/id"},
	} {
		t.Run(c.name, func(t *testing.T) {
			b, diags, err := troupe.ReadBundle(strings.NewReader(`{"troupe":"0.14","kind":"bundle",` + c.bundle + `}`))
			if err != nil || diags != nil {
				t.Fatal(err, diags)
			}
			want := []troupe.Diagnostic{{Code: troupe.CodeDuplicateID, Pointer: c.at}}
			if got := troupe.Check(b, nil); !slices.Equal(got, want) {
				t.Errorf("Check = %v, want %v", got, want)
			}
		})
	}
}

// CheckChanges judges each key of a message's changes, in order, from the
// state before the message, and its diagnostics with the spec's codes are
// those Check gives the message once it is recorded.
func TestCheckChanges(t *testing.T) {
	b, diags, err := troupe.ReadBundle(strings.NewReader(`{"troupe":"0.14","kind":"bundle",
		"characters":[{"id":"a","name":"Ann","description":"d"}],
		"worlds":[{"id":"w","name":"W","description":"d","locations":[{"id":"dock","name":"Dock","description":"d"}]}],
		"scenarios":[{"id":"s","cast":[{},{"character":"a"}],"worlds":["w"],"variables":[
			{"id":"bell","type":"int","max":20,"by":["model"],"maxStep":2,"oneWay":true,"description":"d"},
			{"id":"trust","type":"int","of":"cast","max":100,"by":["model","player"],"maxStep":5,"description":"d"},
			{"id":"inv","type":"int","of":"*","by":["model"],"oneWay":true,"description":"d"},
			{"id":"phase","type":"enum","values":["calm","tense","open"],"by":["model"],"oneWay":true,"description":"d"},
			{"id":"tags","type":"set","by":["model"],"oneWay":true,"description":"d"},
			{"id":"lit","type":"bool","by":["model"],"oneWay":true,"description":"d"},
			{"id":"note","type":"string","description":"d"}]}]}`))
	if err != nil || diags != nil {
		t.Fatal(err, diags)
	}
	// The "*" map is left one key short of the limit.
	inv := make([]string, 254)
	for i := range inv {
		inv[i] = fmt.Sprintf(`"state.inv.k%d":1`, i)
	}
	s, diags, err := troupe.ReadSession(strings.NewReader(`{"troupe":"0.14","kind":"session","scenario":"s","characters":[{"id":"p","name":"Pat","description":"d"}],"players":["p"]}
{"id":"1","from":"player","content":[{"text":"x"}],"changes":[{"by":"player","set":{"state.bell":11,"state.trust.a":98,"state.inv.torch":2,"state.phase":"tense","state.tags":["x"],"state.lit":true,` + strings.Join(inv, ",") + `}}]}`))
	if err != nil || diags != nil {
		t.Fatal(err, diags)
	}
	story, err := troupe.NewStory(b, s)
	if err != nil {
		t.Fatal(err)
	}
	before := story.Apply(story.Initial(), &s.Messages[0])

	at := func(code troupe.Code, pointer string) troupe.Diagnostic {
		return troupe.Diagnostic{Code: code, Pointer: pointer}
	}
	for _, c := range []struct {
		name, changes string
		want          []troupe.Diagnostic
	}{
		{"valid", `[{"by":"model","set":{"state.bell":13,"state.trust.a":93,"state.inv.torch":3,"state.inv.rope":0,"state.phase":"open",
			"state.tags":["y","x"],"state.lit":true,"scene.location":"dock","scene.present":["a","p"]}}]`, nil},
		{"spec", `[{"by":"model","set":{"state.nope":1,"state.trust.zed":1,"state.bell":"x","scene.location":"nowhere","scene.present":["zed"]}}]`, []troupe.Diagnostic{
			at(troupe.CodeUnknownLocation, "/changes/0/set/scene.location"),
			at(troupe.CodeUnknownCharacter, "/changes/0/set/scene.present/0"),
			at(troupe.CodeInvalidValue, "/changes/0/set/state.bell"),
			at(troupe.CodeUnknownVariable, "/changes/0/set/state.nope"),
			at(troupe.CodeUnknownVariable, "/changes/0/set/state.trust.zed"),
		}},
		{"map limit", `[{"by":"model","set":{"state.inv.k255":1}},{"by":"model","set":{"state.inv.k256":1}}]`, []troupe.Diagnostic{
			at(troupe.CodeInvalidValue, "/changes/1/set/state.inv.k256"),
		}},
		// A player's change may set the variables with player in by, and an
		// opening's is held to no rule.
		{"by", `[{"by":"player","set":{"state.bell":12,"state.trust.a":50,"state.note":"x"}},{"by":"model","set":{"state.note":"y"}},{"by":"opening","set":{"state.note":"z","state.bell":5}}]`, []troupe.Diagnostic{
			at(troupe.CodeBy, "/changes/0/set/state.bell"),
			at(troupe.CodeBy, "/changes/0/set/state.note"),
			at(troupe.CodeBy, "/changes/1/set/state.note"),
		}},
		// maxStep bounds the sum of a message's changes by "model", each
		// moving from the value before it, by the normalized value: trust.a
		// falls 8 and rises 10, since 104 is 100. It is reported at the last
		// that sets the key, and a player's changes aren't bounded.
		{"maxStep", `[{"by":"model","set":{"state.bell":12,"state.trust.a":90}},{"by":"player","set":{"state.trust.p":50}},{"by":"model","set":{"state.bell":14,"state.trust.a":104}},
			{"by":"model","set":{"state.lit":true}}]`, []troupe.Diagnostic{
			at(troupe.CodeMaxStep, "/changes/2/set/state.bell"),
		}},
		{"maxStep invalid", `[{"by":"model","set":{"state.bell":20}},{"by":"model","set":{"state.bell":"x"}}]`, []troupe.Diagnostic{
			at(troupe.CodeMaxStep, "/changes/0/set/state.bell"),
			at(troupe.CodeInvalidValue, "/changes/1/set/state.bell"),
		}},
		{"maxStep around a player", `[{"by":"model","set":{"state.trust.a":95}},{"by":"player","set":{"state.trust.a":98}},{"by":"model","set":{"state.trust.a":95}}]`, []troupe.Diagnostic{
			at(troupe.CodeMaxStep, "/changes/2/set/state.trust.a"),
		}},
		{"maxStep after a player", `[{"by":"player","set":{"state.trust.a":50,"state.trust.p":50}},{"by":"model","set":{"state.trust.a":54,"state.trust.p":60}}]`, []troupe.Diagnostic{
			at(troupe.CodeMaxStep, "/changes/1/set/state.trust.p"),
		}},
		// An absent key of a "*" map counts as the default, 0.
		{"oneWay", `[{"by":"model","set":{"state.bell":10,"state.inv.torch":0,"state.inv.rope":-1,"state.phase":"calm","state.tags":["y"],"state.lit":false}}]`, []troupe.Diagnostic{
			at(troupe.CodeOneWay, "/changes/0/set/state.bell"),
			at(troupe.CodeOneWay, "/changes/0/set/state.inv.rope"),
			at(troupe.CodeOneWay, "/changes/0/set/state.inv.torch"),
			at(troupe.CodeOneWay, "/changes/0/set/state.lit"),
			at(troupe.CodeOneWay, "/changes/0/set/state.phase"),
			at(troupe.CodeOneWay, "/changes/0/set/state.tags"),
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			var changes []troupe.Change
			if err := json.Unmarshal([]byte(c.changes), &changes); err != nil {
				t.Fatal(err)
			}
			got := story.CheckChanges(before, changes)
			if !slices.Equal(got, c.want) {
				t.Errorf("CheckChanges = %v, want %v", got, c.want)
			}

			recorded := *s
			recorded.Messages = append(slices.Clip(s.Messages), troupe.Message{ID: "2", Parent: "1", From: "model", Content: []troupe.Part{{Text: "x"}}, Changes: changes})
			var want []troupe.Diagnostic
			for _, d := range got {
				if d.Code != troupe.CodeBy && d.Code != troupe.CodeMaxStep && d.Code != troupe.CodeOneWay {
					d.Line, d.Message = 3, "2"
					want = append(want, d)
				}
			}
			if got := troupe.Check(b, &recorded); !slices.Equal(got, want) {
				t.Errorf("Check = %v, want %v", got, want)
			}
		})
	}
}
