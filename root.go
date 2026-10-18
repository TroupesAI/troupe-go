package troupe

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"maps"
	"math/rand/v2"
	"slices"
	"strings"

	"github.com/TroupesAI/troupe-go/internal/macro"
)

// Root returns the root a session starts with when it plays the scenario's
// opening with the ID opening (Format 11.1): a system message naming the
// opening. Its content is the opening's as written, with each pick replaced
// by the option r chooses, escaped, or empty when the opening has none. When
// the opening has a set, the root records it as its first change, by
// "opening", with the players' characters added to the scene.present it
// sets. The caller gives the root its ID, and its at.
//
// It fails when the scenario has no such opening.
func (st *Story) Root(opening string, r *rand.Rand) (Message, error) {
	sc := st.scope
	i := slices.IndexFunc(sc.scenario.Openings, func(o Opening) bool { return o.ID == opening })
	if i < 0 {
		return Message{}, errors.New("troupe: the scenario has no opening " + opening)
	}
	o := &sc.scenario.Openings[i]
	root := Message{Opening: o.ID, From: "system", Content: []Part{}}
	for _, p := range o.Content {
		p.Text = choosePicks(p.Text, r)
		p.Alt = choosePicks(p.Alt, r)
		root.Content = append(root.Content, p)
	}
	if o.Set != nil {
		set := maps.Clone(o.Set)
		if raw, ok := set["scene.present"]; ok {
			set["scene.present"] = addPlayers(raw, sc.players)
		}
		root.Changes = []Change{{By: "opening", Set: set}}
	}
	return root, nil
}

// choosePicks returns text with each pick replaced by the option r chooses,
// escaped so that it resolves to exactly that option (Format 9, 11.1).
func choosePicks(text string, r *rand.Rand) string {
	tokens, _ := macro.Parse(text)
	var b strings.Builder
	last := 0
	for _, t := range tokens {
		if t.Kind == macro.Pick {
			b.WriteString(text[last:t.Offset])
			b.WriteString(Escape(t.Options[r.IntN(len(t.Options))]))
			last = t.Offset + len(t.Text)
		}
	}
	b.WriteString(text[last:])
	return b.String()
}

// addPlayers returns raw, a scene.present value, with the players it doesn't
// list added. A value that isn't a list of strings is kept as written: it is
// INVALID_VALUE.
func addPlayers(raw jsontext.Value, players []string) jsontext.Value {
	var ids []string
	if json.Unmarshal(raw, &ids) != nil {
		return raw
	}
	n := len(ids)
	for _, p := range players {
		if !slices.Contains(ids, p) {
			ids = append(ids, p)
		}
	}
	if len(ids) == n {
		return raw
	}
	out, err := json.Marshal(ids)
	if err != nil {
		panic(err) // strings that read as I-JSON
	}
	return out
}
