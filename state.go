package troupe

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"maps"
	"slices"
)

// state is the state at a point of a story (Format 8.6), with what else a
// condition or a macro reads there that depends on the path to it (Format
// 9, 10.1).
type state struct {
	location string
	present  []string
	user     string // the player's character {{user}} means; "" for none
	turn     int    // the rounds so far (Format 11.1)
	opening  string // the path's opening; "" for none
	at       string // the message's at, and its parent's; "" for none
	parentAt string
	// appeared holds the characters present in the state at a message on
	// the path (Format 6.1).
	appeared []string
	// values holds each variable in use by its full name: a value from
	// value, or for a keyed variable a map[string]any of them by key.
	values map[string]any
	// entries are the session entries in use, the latest per ID, in the
	// order they were added, each that replaces another in its place (Format
	// 11.3).
	entries []Entry
	// activated holds the round of the latest message on the path that
	// records each entry in activated, by reference, for timing (Format
	// 7.2).
	activated map[string]int
}

// edit is what applying a change replaced, so it can be put back.
type edit struct {
	name string // the full name, or "scene.location" or "scene.present"
	key  string // the key, for a keyed variable
	old  any
	had  bool // whether the key was there; a "*" map's may not be
}

// initial returns sc's initial state: every variable's initial value, no
// location, and the whole cast present except the joiners (Format 8.6).
func (sc *scope) initial() *state {
	st := &state{values: map[string]any{}}
	if len(sc.players) > 0 {
		st.user = sc.players[0]
	}
	for _, id := range sc.cast {
		if !slices.Contains(sc.joiners, id) {
			st.present = append(st.present, id)
		}
	}
	for _, b := range sc.bindings {
		v := b.v
		if !v.Of.keyed() {
			st.values[b.name] = initialValue(b, v.Initial)
			continue
		}
		// A keyed variable's initial is one value for every key, or an object
		// of values by key, where "*" gives the others' (Format 8.3).
		m := map[string]any{}
		var members map[string]jsontext.Value
		if v.Initial.Kind() == '{' {
			if err := json.Unmarshal(v.Initial, &members); err != nil {
				panic(err) // an object that read without PARSE
			}
		}
		for _, k := range sc.keys(v) {
			switch {
			case members == nil:
				m[k] = initialValue(b, v.Initial)
			case members[k] != nil:
				m[k] = initialValue(b, members[k])
			default:
				m[k] = initialValue(b, members["*"])
			}
		}
		st.values[b.name] = m
	}
	return st
}

// initialValue returns raw as a value of b's variable, or its default when
// raw is absent or not valid.
func initialValue(b *binding, raw jsontext.Value) any {
	if raw != nil {
		if x, ok := value(b.v, b.values, raw); ok {
			return x
		}
	}
	return zero(b.v)
}

// apply applies an opening's set or a recorded change to st (Format 8.6),
// adding what it replaces to undo when undo isn't nil. It skips keys and
// values that aren't valid, which a session that passes Check doesn't have.
// It returns the full names of the "*" maps the change leaves with more than
// maxSetMembers keys.
//
// owned holds the full names of st's keyed maps that no other state shares,
// which apply changes in place, and apply adds each map it copies. Copying a
// map once, rather than for each key, keeps applying a change, or a path's
// changes to one state, linear in their keys.
func (sc *scope) apply(st *state, set map[string]jsontext.Value, undo *[]edit, owned map[string]bool) map[string]bool {
	open := map[*binding]bool{} // the "*" maps the change writes to
	for key, raw := range set {
		switch key {
		case "scene.location":
			var loc string
			if json.Unmarshal(raw, &loc) == nil {
				record(undo, edit{name: key, old: st.location, had: true})
				st.location = loc
			}
		case "scene.present":
			var ids []string
			if json.Unmarshal(raw, &ids) == nil {
				record(undo, edit{name: key, old: st.present, had: true})
				// scene.present keeps the cast's characters, once each, in
				// cast order.
				in := index(ids)
				st.present = slices.DeleteFunc(slices.Clone(sc.cast), func(id string) bool {
					_, ok := in[id]
					return !ok
				})
			}
		default:
			b, k, ok := sc.variable(key)
			if !ok {
				continue
			}
			x, ok := value(b.v, b.values, raw)
			if !ok {
				continue
			}
			if k == "" {
				record(undo, edit{name: b.name, old: st.values[b.name], had: true})
				st.values[b.name] = x
				continue
			}
			// A keyed map another state may share is copied, so that state
			// isn't changed.
			m := st.values[b.name].(map[string]any)
			if !owned[b.name] {
				m = maps.Clone(m)
				owned[b.name] = true
			}
			old, had := m[k]
			record(undo, edit{name: b.name, key: k, old: old, had: had})
			if b.v.Of.Any && isZero(b.v, x) {
				// A "*" map's key that holds the default is removed (Format
				// 8.3).
				delete(m, k)
			} else {
				m[k] = x
			}
			st.values[b.name] = m
			if b.v.Of.Any {
				open[b] = true
			}
		}
	}
	over := map[string]bool{}
	for b := range open {
		if len(st.values[b.name].(map[string]any)) > maxSetMembers {
			over[b.name] = true
		}
	}
	return over
}

func record(undo *[]edit, e edit) {
	if undo != nil {
		*undo = append(*undo, e)
	}
}

// revert puts back what undo records, latest first, changing the keyed maps
// in place: apply owned the maps it recorded edits to.
func (st *state) revert(undo []edit) {
	for _, e := range slices.Backward(undo) {
		switch {
		case e.name == "scene.location":
			st.location = e.old.(string)
		case e.name == "scene.present":
			st.present = e.old.([]string)
		case e.key == "":
			st.values[e.name] = e.old
		case e.had:
			st.values[e.name].(map[string]any)[e.key] = e.old
		default:
			delete(st.values[e.name].(map[string]any), e.key)
		}
	}
}

// value returns the value of b's variable in st, at the key k when it is
// keyed: for a key a "*" map doesn't have, the default (Format 8.3).
func (st *state) value(b *binding, k string) any {
	x := st.values[b.name]
	if k == "" {
		return x
	}
	if x, ok := x.(map[string]any)[k]; ok {
		return x
	}
	return zero(b.v)
}

// clone returns a copy of st that applying changes to doesn't change st.
func (st *state) clone() *state {
	c := *st
	c.values = maps.Clone(st.values)
	return &c
}
