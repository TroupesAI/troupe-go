package troupe_test

import (
	"bytes"
	"encoding/json/v2"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"strings"

	"github.com/TroupesAI/troupe-go"
)

// Reading a bundle reports what the file breaks, and Check what it means
// that doesn't hold, here a cast member naming no character.
func ExampleReadBundle() {
	b, diags, err := troupe.ReadBundle(strings.NewReader(`{"troupe": "0.14", "kind": "bundle",
		"characters": [{"id": "kai", "name": "Kai", "description": "A thief."}],
		"scenarios": [{"id": "heist", "cast": [{}, {"character": "kai"}, {"character": "ghost"}]}]}`))
	if err != nil {
		log.Fatal(err)
	}
	diags = append(diags, troupe.Check(b, nil)...)
	for _, d := range diags {
		fmt.Println(d.Code, d.Location())
	}
	// Output:
	// UNKNOWN_CHARACTER /scenarios/0/cast/2/character
}

// A story gives the state at each message from its parent's, and evaluates
// conditions where the next message is being written.
func ExampleStory() {
	b, _, err := troupe.ReadBundle(strings.NewReader(`{"troupe": "0.14", "kind": "bundle",
		"characters": [{"id": "kai", "name": "Kai", "description": "A thief."}],
		"scenarios": [{"id": "heist", "cast": [{}, {"character": "kai"}],
			"variables": [{"id": "bell", "type": "int", "by": ["player"], "description": "The last bell rung."}]}]}`))
	if err != nil {
		log.Fatal(err)
	}
	s, _, err := troupe.ReadSession(strings.NewReader(
		`{"troupe": "0.14", "kind": "session", "scenario": "heist", "characters": [{"id": "mara", "name": "Mara", "description": "A smuggler."}], "players": ["mara"]}
{"id": "1", "from": "player", "content": [{"speaker": "mara", "text": "We wait for the bell."}], "changes": [{"by": "player", "set": {"state.bell": 7}}]}`))
	if err != nil {
		log.Fatal(err)
	}
	story, err := troupe.NewStory(b, s)
	if err != nil {
		log.Fatal(err)
	}

	// A caller that keeps each message's state applies only the next one.
	state := story.Initial()
	for i := range s.Messages {
		state = story.Apply(state, &s.Messages[i])
	}
	bell, _ := state.Get("state.bell")
	fmt.Println("bell:", bell)

	// The message being written is for Kai, after message 1.
	point := story.PointAt(story.Initial(), s.Messages, "kai")
	ok, err := point.Eval("state.bell >= 7 && hit('bell')", "heist", 4)
	fmt.Println(ok, err)
	// Output:
	// bell: 7
	// true <nil>
}

// A session starts from an opening's root. Each turn, the caller restores the
// stored state before the messages a scan reaches, builds the point after
// them, and gets the entries active for the message being written.
func ExamplePoint_Active() {
	b, _, err := troupe.ReadBundle(strings.NewReader(`{"troupe": "0.14", "kind": "bundle",
		"characters": [{"id": "kai", "name": "Kai", "description": "A thief.",
			"entries": [{"id": "fear", "text": "Kai fears heights.", "keys": ["/roofs?/"], "sticky": 2}]}],
		"scenarios": [{"id": "heist", "cast": [{}, {"character": "kai"}],
			"openings": [{"id": "rooftop", "set": {"scene.present": ["kai"]},
				"content": [{"text": "Rain on the {{pick::roof::slates}}."}]}]}]}`))
	if err != nil {
		log.Fatal(err)
	}
	// The player's character, as a person typed it: its description is
	// escaped, since macros resolve there.
	header, err := json.Marshal(troupe.SessionHeader{
		Troupe:   troupe.Version,
		Scenario: "heist",
		Characters: []troupe.Character{{
			ID: "mara", Name: "Mara", Description: troupe.Escape("A smuggler {{who never pays}}."),
		}},
		Players: []string{"mara"},
	})
	if err != nil {
		log.Fatal(err)
	}
	s, _, err := troupe.ReadSession(bytes.NewReader(header))
	if err != nil {
		log.Fatal(err)
	}
	story, err := troupe.NewStory(b, s)
	if err != nil {
		log.Fatal(err)
	}
	r := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // G404: a seeded source makes the example reproducible

	root, err := story.Root("rooftop", r)
	if err != nil {
		log.Fatal(err)
	}
	root.ID = "1"
	fmt.Println(root.Content[0].Text, string(root.Changes[0].Set["scene.present"]))
	stored, err := json.Marshal(story.Apply(story.Initial(), &root))
	if err != nil {
		log.Fatal(err)
	}

	// Message 2 is written for Kai, after the root, which the scan reaches.
	reply := troupe.Message{ID: "2", Parent: "1", From: "model", Char: "kai", Content: []troupe.Part{{Text: "Kai grips the ledge."}}}
	for _, a := range story.PointAt(story.Initial(), []troupe.Message{root}, "kai").Active(r) {
		fmt.Println(a.Owner+"/"+a.Entry.ID, a.Kept)
		if !a.Kept {
			reply.Activated = append(reply.Activated, a.Owner+"/"+a.Entry.ID)
		}
	}

	// Message 3 is written for Kai too, after message 2, from the root's
	// stored state.
	before, err := story.UnmarshalState(stored)
	if err != nil {
		log.Fatal(err)
	}
	for _, a := range story.PointAt(before, []troupe.Message{reply}, "kai").Active(r) {
		fmt.Println(a.Owner+"/"+a.Entry.ID, a.Kept)
	}
	// Output:
	// Rain on the roof. ["kai","mara"]
	// kai/fear false
	// kai/fear true
}

// On import, a file a bundle holds as a data: URI can be stored apart, and
// its field set to the file's path, with every other byte kept as written.
func ExampleReplaceFileFields() {
	doc := []byte(`{"troupe": "0.14", "kind": "bundle", "cover": "data:image/png;base64,iVBORw0KGgo=", "ext": {"x.y": [1, 2.0]}}`)
	b, _, err := troupe.ReadBundle(bytes.NewReader(doc))
	if err != nil {
		log.Fatal(err)
	}
	paths := map[string]string{}
	for f := range b.FileFields() {
		if f.MediaType != "" {
			fmt.Println(f.Pointer, f.MediaType, len(f.Data), "bytes")
			paths[f.Pointer] = "images/cover.png"
		}
	}
	doc, err = troupe.ReplaceFileFields(doc, paths)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(doc))
	// Output:
	// /cover image/png 8 bytes
	// {"troupe": "0.14", "kind": "bundle", "cover": "images/cover.png", "ext": {"x.y": [1, 2.0]}}
}

// A package's bundle.json, stored apart from its ZIP, reads back given the
// paths of the package's files, such as Package.Files.
func ExampleReadPackageBundle() {
	doc := `{"troupe": "0.14", "kind": "bundle", "cover": "images/cover.png"}`
	b, diags, err := troupe.ReadPackageBundle(strings.NewReader(doc), []string{"images/cover.png"})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(b.Cover, len(diags))

	// Without that file, the path names nothing.
	_, diags, err = troupe.ReadPackageBundle(strings.NewReader(doc), nil)
	if err != nil {
		log.Fatal(err)
	}
	for _, d := range diags {
		fmt.Println(d.Code, d.File, d.Location())
	}
	// Output:
	// images/cover.png 0
	// PARSE bundle.json /cover
}

// An editor renames a character, and every reference to it, with a JSON
// Patch, which a patch library applies to the stored bundle.json.
func ExampleBundle_RenameID() {
	b, _, err := troupe.ReadBundle(strings.NewReader(`{"troupe": "0.14", "kind": "bundle",
		"characters": [{"id": "kai_moreau", "name": "Kai Moreau", "description": "A thief."}],
		"scenarios": [{"id": "heist", "cast": [{}, {"character": "kai_moreau"}],
			"entries": [{"id": "watch", "text": "The guard is near.", "when": "'kai_moreau' in scene.present"}]}]}`))
	if err != nil {
		log.Fatal(err)
	}
	if err := b.CheckID("/characters", "kai_moreau"); errors.Is(err, troupe.ErrIDTaken) {
		fmt.Println("taken")
	}
	patch, left, err := b.RenameID("/characters/0", "kai")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(patch), left)
	// Output:
	// taken
	// [{"op":"add","path":"/characters/0/id","value":"kai"},{"op":"replace","path":"/scenarios/0/cast/1/character","value":"kai"},{"op":"replace","path":"/scenarios/0/entries/0/when","value":"'kai' in scene.present"}] []
}

// An editor offers the keys an opening's set can write, each with the
// variable that says how to enter its value.
func ExampleStory_StateKeys() {
	b, _, err := troupe.ReadBundle(strings.NewReader(`{"troupe": "0.14", "kind": "bundle",
		"characters": [{"id": "kai", "name": "Kai", "description": "A thief.",
			"variables": [{"id": "nerve", "type": "int", "min": 0, "max": 5, "description": "How steady Kai is."}]}],
		"scenarios": [{"id": "heist", "cast": [{}, {"character": "kai"}],
			"variables": [
				{"id": "bell", "type": "int", "description": "The last bell rung."},
				{"id": "trust", "type": "int", "of": "cast", "description": "Trust in the player."},
				{"id": "loot", "type": "int", "of": "*", "description": "What the crew carries."}]}]}`))
	if err != nil {
		log.Fatal(err)
	}
	story, err := troupe.NewScenarioStory(b, "heist")
	if err != nil {
		log.Fatal(err)
	}
	for _, k := range story.StateKeys() {
		switch {
		case k.Variable == nil:
			fmt.Println(k.Name)
		case k.Key == "*":
			fmt.Println(k.Name+".<any ID>", k.Variable.Type, k.Owner)
		default:
			fmt.Println(k.Name, k.Variable.Type, k.Owner)
		}
	}
	// Output:
	// scene.location
	// scene.present
	// state.bell int heist
	// state.trust.kai int heist
	// state.loot.<any ID> int heist
	// state.kai.nerve int kai
}
