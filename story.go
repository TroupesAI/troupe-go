package troupe

import (
	"errors"
	"iter"
	"maps"
	"slices"
	"strings"
)

// Story is a bundle's scenario as a session plays it, or played on its own.
// The states it gives are its own: a method that takes a State takes one this
// story gave.
type Story struct {
	scope    *scope
	messages []Message
	index    map[string]int // each message ID's first line, as an index into messages
	initial  *state
	conditions
}

// NewStory returns the story of session s, played with bundle b. Without s, it
// returns the story of b's first scenario played on its own, as
// NewScenarioStory does. A session without messages, such as one read from
// its header alone, serves to compute states with Apply.
//
// It fails when b has no scenario, or s's scenario isn't one of b's. b and s
// are documents that pass Check; the results for others are only bounded,
// since the spec gives them no meaning (Format 15.1).
func NewStory(b *Bundle, s *Session) (*Story, error) {
	if s == nil {
		if len(b.Scenarios) == 0 {
			return nil, errors.New("troupe: the bundle has no scenario")
		}
		return newStory(newScope(b, nil, 0), nil), nil
	}
	i := scenarioIndex(b, s.Header.Scenario)
	if i < 0 {
		return nil, errors.New("troupe: the session's scenario isn't in the bundle")
	}
	return newStory(newScope(b, s, i), s.Messages), nil
}

// NewScenarioStory returns the story of b's scenario with the ID scenario,
// played on its own: its seats are unfilled, so it has no players
// (conformance README section 2). It is the scenario as a person starting a
// session sees it, before choosing whom to play.
//
// It fails when b has no such scenario.
func NewScenarioStory(b *Bundle, scenario string) (*Story, error) {
	i := scenarioIndex(b, scenario)
	if i < 0 {
		return nil, errors.New("troupe: the bundle has no scenario " + scenario)
	}
	return newStory(newScope(b, nil, i), nil), nil
}

// newStory returns the story of sc, with a session's messages.
func newStory(sc *scope, messages []Message) *Story {
	st := &Story{scope: sc, messages: messages, index: map[string]int{}, initial: sc.initial()}
	for j, m := range messages {
		if _, ok := st.index[m.ID]; !ok {
			st.index[m.ID] = j
		}
	}
	return st
}

// Initial returns the state before any message (Format 8.6).
func (st *Story) Initial() State {
	return State{st.scope, st.initial}
}

// Apply returns the state after a message, given the state of this story
// before it: its parent's, or the initial state for a root. That is the state
// after the message's changes (Format 8.6), with what else depends on the
// path, such as the rounds so far, the player's character {{user}} means, the
// session entries in use, the rounds entries were recorded as activated in
// and who has been present (Format 6.1, 7.2, 9, 10.1, 11.3). Applying the
// messages of a path in turn gives the state at its last, so a caller that
// keeps each message's state never replays a path.
func (st *Story) Apply(before State, m *Message) State {
	s := before.s.clone()
	st.apply(s, m)
	return State{st.scope, s}
}

// apply applies m to s: its changes, in order, and who they leave present;
// for a player message, a round, and with a speaking part, the first such
// part's speaker as {{user}} (Format 9, 11.1); for a root, its opening; m's
// at; its entries, then its activated, which its own entries may be in
// (Format 11.3).
func (st *Story) apply(s *state, m *Message) {
	for _, c := range m.Changes {
		st.scope.apply(s, c.Set, nil)
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
		s.turn++
		if i := slices.IndexFunc(m.Content, func(p Part) bool { return p.Speaker != "" }); i >= 0 {
			s.user = m.Content[i].Speaker
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
			activated[ref] = s.turn
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
func (st *Story) StateAt(id string) (State, error) {
	i, ok := st.index[id]
	if !ok {
		return State{}, errors.New("troupe: no message " + id)
	}
	var path []int
	for {
		path = append(path, i)
		parent, ok := st.index[st.messages[i].Parent]
		// A parent is on an earlier line (Format 11.1); any other makes a
		// root.
		if !ok || parent >= i {
			break
		}
		i = parent
	}
	s := st.initial.clone()
	for _, i := range slices.Backward(path) {
		st.apply(s, &st.messages[i])
	}
	return State{st.scope, s}, nil
}

// State is the state at a point of a story (Format 8.6). The zero State holds
// nothing.
type State struct {
	scope *scope
	s     *state
}

// Get returns the value at key, a key as a change writes it (Format 8.5):
// "scene.location", "scene.present", a variable's full name, or a keyed
// variable's full name and one of its keys, such as "state.trust.kai". The
// value is a bool, an int64, a string, or a []string for a set and for
// scene.present. It reports false when the state holds nothing at key, as for
// a key a "*" map doesn't have.
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

// All yields every key the state holds with its value, as Get gives them:
// scene.location and scene.present, then each variable in use, the
// scenario's, its worlds' and its characters', with a keyed variable's keys in
// their order: the cast's, the list's, or for a "*" map, code point order
// (Format 8.3).
func (s State) All() iter.Seq2[string, any] {
	return func(yield func(string, any) bool) {
		if s.s == nil {
			return
		}
		if !yield("scene.location", s.s.location) || !yield("scene.present", slices.Clone(s.s.present)) {
			return
		}
		for _, b := range s.scope.bindings {
			x := s.s.values[b.name]
			m, keyed := x.(map[string]any)
			if !keyed {
				if !yield(b.name, copyValue(x)) {
					return
				}
				continue
			}
			keys := s.scope.keys(b.v)
			if b.v.Of.Any {
				keys = slices.Sorted(maps.Keys(m))
			}
			for _, k := range keys {
				if !yield(b.name+"."+k, copyValue(m[k])) {
					return
				}
			}
		}
	}
}

// copyValue returns x, with a set copied, so callers can't change the state.
func copyValue(x any) any {
	if s, ok := x.([]string); ok {
		return slices.Clone(s)
	}
	return x
}
