package troupe

import (
	"errors"
	"iter"
	"maps"
	"slices"
	"strings"
)

// Story is a bundle's scenario as a session plays it, or played on its own.
type Story struct {
	scope    *scope
	messages []Message
	index    map[string]int // each message ID's first line, as an index into messages
	initial  *state
	conditions
}

// NewStory returns the story of session s, played with bundle b. Without s, it
// returns the story of b's first scenario, whose seats are unfilled, so it has
// no players (conformance README section 2). A session without messages, such
// as one read from its header alone, serves to compute states with Apply.
//
// b and s are documents that pass Check. The results for others are only
// bounded: the spec gives them no meaning (Format 15.1).
func NewStory(b *Bundle, s *Session) (*Story, error) {
	st := &Story{index: map[string]int{}}
	if s == nil {
		if len(b.Scenarios) == 0 {
			return nil, errors.New("troupe: the bundle has no scenario")
		}
		st.scope = newScope(b, nil, 0)
	} else {
		i := scenarioIndex(b, s.Header.Scenario)
		if i < 0 {
			return nil, errors.New("troupe: the session's scenario isn't in the bundle")
		}
		st.scope = newScope(b, s, i)
		st.messages = s.Messages
		for j, m := range s.Messages {
			if _, ok := st.index[m.ID]; !ok {
				st.index[m.ID] = j
			}
		}
	}
	st.initial = st.scope.initial()
	return st, nil
}

// Initial returns the state before any message (Format 8.6).
func (st *Story) Initial() State {
	return State{st.scope, st.initial}
}

// Apply returns the state after a message, given the state of this story
// before it: its parent's, or the initial state for a root. That is the state
// after the message's changes (Format 8.6), with what else conditions and
// macros read that depends on the path, such as the rounds so far and the
// player's character {{user}} means (Format 9, 10.1). Applying the messages
// of a path in turn gives the state at its last, so a caller that keeps each
// message's state never replays a path.
func (st *Story) Apply(before State, m *Message) State {
	s := before.s.clone()
	st.apply(s, m)
	return State{st.scope, s}
}

// apply applies m to s: its changes, in order; for a player message, a
// round, and with a speaking part, the first such part's speaker as {{user}}
// (Format 9, 11.1); for a root, its opening; and m's at.
func (st *Story) apply(s *state, m *Message) {
	for _, c := range m.Changes {
		st.scope.apply(s, c.Set, nil)
	}
	if m.Parent == "" {
		s.opening = m.Opening
	}
	s.parentAt, s.at = s.at, m.At
	if m.From != "player" {
		return
	}
	s.turn++
	if i := slices.IndexFunc(m.Content, func(p Part) bool { return p.Speaker != "" }); i >= 0 {
		s.user = m.Content[i].Speaker
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
