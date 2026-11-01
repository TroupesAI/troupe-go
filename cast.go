package troupe

import "slices"

// Member is a member of a play's cast (Format 6.1, 11.2).
type Member struct {
	// Character is the member's character, from the story or the session's
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
	// {{char}}, and the scenario as owner (Format 9). State.Spoilers tells
	// whether a spoiler still is one.
	Notes   string
	Spoiler bool
}

// Spoilers returns the characters of the cast members marked spoiler that are
// still spoilers for players at s, in cast order: those that no state at a
// message on its path, the root's included, has present (Format 6.1). The
// cast starts present, so only a root that leaves a member out of
// scene.present, such as through an opening's set, keeps it a spoiler.
func (s State) Spoilers() []string {
	if s.s == nil {
		return nil
	}
	var out []string
	for _, m := range s.scope.members {
		if m.of != nil && m.of.Spoiler && slices.Contains(s.scope.cast, m.id) && !slices.Contains(s.s.appeared, m.id) {
			out = append(out, m.id)
		}
	}
	return out
}

// Cast returns the play's cast, in cast order (Format 6.1, 11.2). In a
// session, that is the scenario cast with its seats filled by the players'
// header characters, then the new seats, then the joiners; a seat no player
// fills is left out. A scenario played on its own has no players, so it lists
// every member as written, seats included, with the default of one seat and
// every story character: what a person starting a session can choose from.
// Its seats aren't in the cast its states have (conformance README section
// 2).
func (pl *Play) Cast() []Member {
	sc := pl.scope
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
// story world when its worlds is absent, then the session's (Format 6.4).
func (pl *Play) Worlds() []*World {
	out := make([]*World, len(pl.scope.worldIDs))
	for i, id := range pl.scope.worldIDs {
		out[i] = pl.scope.worlds[id]
	}
	return out
}

// Mode returns the scenario's mode: "characters" or "narrator" (Format 6.2).
func (pl *Play) Mode() string {
	if pl.scope.scenario.Mode == "" {
		return "characters"
	}
	return pl.scope.scenario.Mode
}

// NarratorName returns what text calls the narrator: the scenario's
// narratorName, or "the narrator" (Format 6).
func (pl *Play) NarratorName() string {
	return pl.scope.narrator()
}
