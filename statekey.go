package troupe

// StateKey is a key a story's changes can write (Format 8.5).
type StateKey struct {
	// Name is the key as a change writes it: "scene.location",
	// "scene.present", a variable's full name (Format 8.2), or a keyed
	// variable's full name and one of its keys, such as "state.trust.kai".
	// For a "*" map, whose keys can't be listed, it is the variable's full
	// name.
	Name string
	// Key is a keyed variable's key: a cast character's ID for "cast", or one
	// of its list, and "*" for a "*" map, where any author ID other than char
	// and user is a key, written after Name and a dot (Format 8.3). It is ""
	// for the scene and for a variable that isn't keyed.
	Key string
	// Variable is the variable the key writes, which gives its type and
	// values; nil for the scene: scene.location, a location ID or "", and
	// scene.present, the characters present (Format 8.4).
	Variable *Variable
	// Owner is the variable's owner, the scenario's, a world's or a
	// character's ID; "" for the scene.
	Owner string
}

// StateKeys returns the keys the story's changes and its openings' set can
// write (Format 6.3, 8.5), in the order State.All yields them:
// scene.location and scene.present, then each variable in use, the
// scenario's, its worlds' and its characters', with a keyed variable's keys in
// their order. A "cast" variable's keys are the characters of the cast the
// story's states have, so a scenario played on its own leaves out its seats'
// (conformance README section 2).
//
// It offers what is valid, and Check still judges what is written: for a
// draft that fails Check, it gives the keys of what reads, each once.
func (st *Story) StateKeys() []StateKey {
	sc := st.scope
	out := []StateKey{{Name: "scene.location"}, {Name: "scene.present"}}
	for _, b := range sc.bindings {
		switch {
		case !b.v.Of.keyed():
			out = append(out, StateKey{Name: b.name, Variable: b.v, Owner: b.owner})
		case b.v.Of.Any:
			out = append(out, StateKey{Name: b.name, Key: "*", Variable: b.v, Owner: b.owner})
		default:
			// A draft's cast or list may hold a key twice (DUPLICATE_ID,
			// SCHEMA).
			seen := map[string]bool{}
			for _, k := range sc.keys(b.v) {
				if !seen[k] {
					seen[k] = true
					out = append(out, StateKey{Name: b.name + "." + k, Key: k, Variable: b.v, Owner: b.owner})
				}
			}
		}
	}
	return out
}
