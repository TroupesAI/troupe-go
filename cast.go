package troupe

import "slices"

// Member is a member of a story's cast (Format 6.1, 11.2).
type Member struct {
	// Character is the member's character, from the bundle or the session's
	// header; nil for a seat without one, which only a scenario played on
	// its own has.
	Character *Character
	// Seat reports whether a person fills the member: a member without a
	// character, filled by a character the person brings, one marked player,
	// or a new seat after the scenario cast.
	Seat bool
	// Player reports whether a person plays the character: players lists
	// it. A model plays the others.
	Player bool
	// Notes and Spoiler are the scenario's for the member; a new seat and a
	// joiner have none. Notes render with the member's character as
	// {{char}}, and the scenario as owner (Format 9).
	Notes   string
	Spoiler bool
}

// Cast returns the story's cast, in cast order (Format 6.1, 11.2). In a
// session, that is the scenario cast with its seats filled by the players'
// header characters, then the new seats, then the joiners; a seat no player
// fills is left out. A scenario played on its own has no players, so it lists
// every member as written, seats included, with the default of one seat and
// every bundle character: what a person starting a session can choose from.
// Its seats aren't in the cast its states have (conformance README section
// 2).
func (st *Story) Cast() []Member {
	sc := st.scope
	var out []Member
	for _, m := range sc.members {
		c := sc.all.characters[m.id]
		if c == nil && m.id != "" {
			continue // UNKNOWN_CHARACTER
		}
		member := Member{Character: c, Seat: m.seat, Player: slices.Contains(sc.players, m.id)}
		if m.of != nil {
			member.Notes, member.Spoiler = m.of.Notes, m.of.Spoiler
		}
		out = append(out, member)
	}
	return out
}

// Worlds returns the worlds in use, in order: the scenario's worlds, or every
// bundle world when its worlds is absent, then the session's (Format 6.4).
func (st *Story) Worlds() []*World {
	out := make([]*World, len(st.scope.worldIDs))
	for i, id := range st.scope.worldIDs {
		out[i] = st.scope.worlds[id]
	}
	return out
}

// Mode returns the scenario's mode: "characters" or "narrator" (Format 6.2).
func (st *Story) Mode() string {
	if st.scope.scenario.Mode == "" {
		return "characters"
	}
	return st.scope.scenario.Mode
}

// Narrator returns what text calls the narrator: the scenario's narrator, or
// "the narrator" (Format 6).
func (st *Story) Narrator() string {
	return st.scope.narrator()
}
