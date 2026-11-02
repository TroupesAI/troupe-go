package troupe

// StateKey is a state key a play's changes can write (Format 8.5).
type StateKey struct {
	// Name is the state key: "scene.location", "scene.present", a variable's
	// full name (Format 8.2), or a keyed variable's full name and one of its
	// keys, such as "state.trust.kai". For a "*" map, whose keys can't be
	// listed, it is the variable's full name.
	Name string
	// Key is a keyed variable's key: a cast character's ID for "cast", or one
	// of its list, and "*" for a "*" map, where any author ID other than char
	// and player is a key, written after Name and a dot (Format 8.3). It is ""
	// for the scene and for a variable that isn't keyed.
	Key string
	// Variable is the variable the state key writes, which gives its type and
	// values; nil for the scene: scene.location, a location ID or "", and
	// scene.present, the characters present (Format 8.4).
	Variable *Variable
	// Owner is the variable's owner, the scenario's, a world's or a
	// character's ID; "" for the scene.
	Owner string
}

// StateKeys returns the state keys the play's changes and its openings' set
// can write (Format 6.3, 8.5): scene.location and scene.present, then each
// variable in use, the scenario's, its worlds' and its characters', with a
// keyed variable's keys in their order, as State.All yields them. In a
// scenario played on its own, a "cast" variable's keys also include the
// characters of seats marked player, which its openings may set though its
// states leave them out (Format 8.5); a root leaves them out of a session
// where no player fills the seat (Format 11.1).
//
// It offers what is valid, and Check still judges what is written: for a
// draft that fails Check, it gives the state keys of what reads, each once.
func (pl *Play) StateKeys() []StateKey {
	sc := pl.scope
	out := []StateKey{{Name: "scene.location"}, {Name: "scene.present"}}
	for _, b := range sc.bindings {
		switch {
		case !b.v.Per.keyed():
			out = append(out, StateKey{Name: b.name, Variable: b.v, Owner: b.owner})
		case b.v.Per.Any:
			out = append(out, StateKey{Name: b.name, Key: "*", Variable: b.v, Owner: b.owner})
		default:
			// A draft's cast or list may hold a key twice (DUPLICATE_ID,
			// SCHEMA).
			seen := map[string]bool{}
			keys := b.v.Per.List
			if b.v.Per.Cast {
				keys = sc.openingCast
			}
			for _, k := range keys {
				if !seen[k] {
					seen[k] = true
					out = append(out, StateKey{Name: b.name + "." + k, Key: k, Variable: b.v, Owner: b.owner})
				}
			}
		}
	}
	return out
}
