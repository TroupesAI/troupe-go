package troupe_test

import (
	"fmt"
	"log"
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
