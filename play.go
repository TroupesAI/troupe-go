package troupe

import (
	"errors"
	"iter"
	"maps"
	"slices"
	"strings"
)

// Play is a story's scenario as a session plays it, or played on its own.
// The states it gives are its own: a method that takes a State takes one this
// play gave. Every method about a message other than StateAt takes the state
// before it first, so a caller always passes the parent's state.
type Play struct {
	scope    *scope
	messages []Message
	index    map[string]int // each message ID's first line, as an index into messages
	initial  *state
	conditions
}

// NewPlay returns the play of session s, played with story st. Without s, it
// returns the play of st's first scenario on its own, as NewScenarioPlay
// does. A session without messages, such as one read from its header alone,
// serves to compute states with Apply.
//
// It fails when st has no scenario, or s's scenario isn't one of st's. st and
// s are documents that conform; the results for others are only bounded,
// since the spec gives them no meaning (Format 15.1).
func NewPlay(st *Story, s *Session) (*Play, error) {
	if s == nil {
		if len(st.Scenarios) == 0 {
			return nil, errors.New("troupe: the story has no scenario")
		}
		return newPlay(newScope(st, nil, 0), nil), nil
	}
	i := scenarioIndex(st, s.Header.Scenario)
	if i < 0 {
		return nil, errors.New("troupe: the session's scenario isn't in the story")
	}
	return newPlay(newScope(st, s, i), s.Messages), nil
}

// NewScenarioPlay returns the play of st's scenario with the ID scenario,
// played on its own: its seats are unfilled, so it has no players
// (conformance README section 2). It is the scenario as a person starting a
// session sees it, before choosing whom to play.
//
// It fails when st has no such scenario.
func NewScenarioPlay(st *Story, scenario string) (*Play, error) {
	i := scenarioIndex(st, scenario)
	if i < 0 {
		return nil, errors.New("troupe: the story has no scenario " + scenario)
	}
	return newPlay(newScope(st, nil, i), nil), nil
}

// newPlay returns the play of sc, with a session's messages.
func newPlay(sc *scope, messages []Message) *Play {
	pl := &Play{scope: sc, messages: messages, index: map[string]int{}, initial: sc.initial()}
	for j, m := range messages {
		if _, ok := pl.index[m.ID]; !ok {
			pl.index[m.ID] = j
		}
	}
	return pl
}

// Initial returns the state before any message (Format 8.6).
func (pl *Play) Initial() State {
	return State{pl.scope, pl.initial}
}

// Apply returns the state after a message, given the state of this play
// before it: its parent's, or the initial state for a root. That is the state
// after the message's changes (Format 8.6), with what else depends on the
// path, such as the rounds so far, the player's character {{player}} means,
// the session entries in use, the rounds entries were recorded as activated
// in and who has been present (Format 6.1, 7.2, 9, 10.1, 11.3). Applying the
// messages of a path in turn gives the state at its last, so a caller that
// keeps each message's state never replays a path.
func (pl *Play) Apply(before State, m *Message) State {
	s := before.s.clone()
	pl.apply(s, m, map[string]bool{})
	return State{pl.scope, s}
}

// apply applies m to s: its changes, in order, and who they leave present;
// for a player message, a round, and with a speaking part, the first such
// part's speaker as {{player}} (Format 9, 11.1); for a root, its opening; m's
// at; its entries, then its activated, which its own entries may be in
// (Format 11.3). owned is as scope.apply takes it.
func (pl *Play) apply(s *state, m *Message, owned map[string]bool) {
	for _, c := range m.Changes {
		pl.scope.apply(s, c.Set, nil, owned)
	}
	for _, id := range s.present {
		if !slices.Contains(s.appeared, id) {
			// Clipped: states cloned from one parent share this array.
			s.appeared = append(slices.Clip(s.appeared), id)
		}
	}
	if m.Parent == "" {
		s.opening = m.Opening
	}
	s.parentAt, s.at = s.at, m.At
	if m.From == "player" {
		s.round++
		if i := slices.IndexFunc(m.Content, func(p Part) bool { return p.Speaker != "" }); i >= 0 {
			s.player = m.Content[i].Speaker
		}
	}
	if len(m.Entries) > 0 {
		// Replaced, not changed in place, as values are.
		s.entries = slices.Clone(s.entries)
		for _, e := range m.Entries {
			s.addEntry(e)
		}
	}
	if len(m.Activated) > 0 {
		activated := make(map[string]int, len(s.activated)+len(m.Activated))
		maps.Copy(activated, s.activated)
		for _, ref := range m.Activated {
			activated[ref] = s.round
		}
		s.activated = activated
	}
}

// addEntry adds a session entry: a later entry with an ID replaces the
// earlier, and one that is disabled removes it (Format 11.3).
func (s *state) addEntry(e Entry) {
	i := slices.IndexFunc(s.entries, func(x Entry) bool { return x.ID == e.ID })
	switch {
	case i < 0 && !e.Disabled:
		s.entries = append(s.entries, e)
	case i >= 0 && e.Disabled:
		s.entries = slices.Delete(s.entries, i, i+1)
	case i >= 0:
		s.entries[i] = e
	}
}

// StateAt returns the state at the session's message with the ID id, after
// its changes: those of every message on the path from its root, in order
// (Format 8.6).
func (pl *Play) StateAt(id string) (State, error) {
	i, ok := pl.index[id]
	if !ok {
		return State{}, errors.New("troupe: no message " + id)
	}
	var path []int
	for {
		path = append(path, i)
		parent, ok := pl.index[pl.messages[i].Parent]
		// A parent is on an earlier line (Format 11.1); any other makes a
		// root.
		if !ok || parent >= i {
			break
		}
		i = parent
	}
	s, owned := pl.initial.clone(), map[string]bool{}
	for _, i := range slices.Backward(path) {
		pl.apply(s, &pl.messages[i], owned)
	}
	return State{pl.scope, s}, nil
}

// State is the state at a point of a play (Format 8.6). The zero State holds
// nothing.
type State struct {
	scope *scope
	s     *state
}

// Get returns the value at key, a state key (Format 8.5): "scene.location",
// "scene.present", a variable's full name, or a keyed variable's full name and
// one of its keys, such as "state.trust.kai". The value is a bool, an int64, a
// string, or a []string for a set and for scene.present. It reports false when
// the state holds nothing at key, as for a key a "*" map doesn't have.
func (s State) Get(key string) (any, bool) {
	if s.s == nil {
		return nil, false
	}
	switch key {
	case "scene.location":
		return s.s.location, true
	case "scene.present":
		return slices.Clone(s.s.present), true
	}
	if x, ok := s.s.values[key]; ok {
		if _, keyed := x.(map[string]any); !keyed {
			return copyValue(x), true
		}
		return nil, false
	}
	i := strings.LastIndexByte(key, '.')
	if i < 0 {
		return nil, false
	}
	m, ok := s.s.values[key[:i]].(map[string]any)
	if !ok {
		return nil, false
	}
	x, ok := m[key[i+1:]]
	return copyValue(x), ok
}

// All yields every state key the state holds, with its value as Get gives
// it: scene.location and scene.present, then each variable in use, the
// scenario's, its worlds' and its characters', with a keyed variable's keys
// in their order: the cast's, the list's, or for a "*" map, those it holds,
// in code point order (Format 8.3).
func (s State) All() iter.Seq2[StateKey, any] {
	return func(yield func(StateKey, any) bool) {
		if s.s == nil {
			return
		}
		if !yield(StateKey{Name: "scene.location"}, s.s.location) || !yield(StateKey{Name: "scene.present"}, slices.Clone(s.s.present)) {
			return
		}
		for _, b := range s.scope.bindings {
			x := s.s.values[b.name]
			m, keyed := x.(map[string]any)
			if !keyed {
				if !yield(StateKey{Name: b.name, Variable: b.v, Owner: b.owner}, copyValue(x)) {
					return
				}
				continue
			}
			keys := s.scope.keys(b.v)
			if b.v.Per.Any {
				keys = slices.Sorted(maps.Keys(m))
			}
			for _, k := range keys {
				if !yield(StateKey{Name: b.name + "." + k, Key: k, Variable: b.v, Owner: b.owner}, copyValue(m[k])) {
					return
				}
			}
		}
	}
}

// Within returns the scene's location, then its ancestors, innermost first,
// as scene.within lists them (Format 8.5, 10.1): locations of the story or
// session, whether or not their worlds are in use. There are none when the
// scene has no location.
func (s State) Within() []*Location {
	if s.s == nil {
		return nil
	}
	locations := s.scope.all.locations
	var out []*Location
	// A chain holds each location once, so the count cuts short only the walk
	// of a cycle, which fails Check. Searching the chain for each parent
	// would end it too, but is quadratic in the chain's length, which Format
	// 14 doesn't bound.
	for l := locations[s.s.location]; l != nil && len(out) < len(locations); l = locations[l.Parent] {
		out = append(out, l)
	}
	return out
}

// copyValue returns x, with a set copied, so callers can't change the state.
func copyValue(x any) any {
	if s, ok := x.([]string); ok {
		return slices.Clone(s)
	}
	return x
}
