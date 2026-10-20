package troupe

import (
	"math/rand/v2"
	"slices"
)

// ActiveEntry is an entry active for the message being written (Format 7.2).
type ActiveEntry struct {
	// Owner is the entry's owner: a character, world or scenario ID, or
	// "session" for a session entry. Render and Eval take it as the owner,
	// and the entry's reference is Owner, "/" and its ID (Format 2.4).
	Owner string
	Entry Entry
	// Kept reports whether the entry's sticky keeps it active from a message
	// that records it, so the message being written doesn't record it in
	// activated again (Format 11.4).
	Kept bool
}

// Active returns the entries active for the message being written at p
// (Format 7.2), in the order of their owners, the scenario, its worlds, the
// characters of the cast and the session, and each owner's in its order. The
// entries in use are the scenario's, its worlds', those of the characters
// present or the message is written for, and the session's; of these, those
// whoever the message is for knows (Format 7.4).
//
// An entry with keys activates when one matches its scanned text, and one
// without keys or pull always does, while its when holds. An entry with pull
// and no keys activates only by retrieval, which is the caller's. The rounds
// of the messages that record an entry in activated time it: sticky keeps it
// active, and once and cooldown keep it from activating again. Of the entries
// that would activate, chance lets each through, and then group keeps one of
// each owner's group, of the highest priority, chosen by weight among equals,
// where an entry sticky keeps wins against equals that aren't kept.
// r draws for chance and weight, so activation is reproducible from the
// caller's source.
//
// Which active entries reach the model is the caller's to choose, by their
// priority, and where each goes by its position or depth (Format 7.3). The
// message being written records those that reach it in activated, apart from
// those Kept; one whose text renders as nothing or only whitespace reaches
// nothing (Format 11.4).
func (p *Point) Active(r *rand.Rand) []ActiveEntry {
	var active []ActiveEntry
	for _, a := range p.inUse() {
		if p.activates(&a, r) {
			active = append(active, a)
		}
	}
	return compete(active, r)
}

// inUse returns the entries in use at p that whoever the message being
// written is for knows, and that aren't disabled (Format 6.4, 7.1, 7.4).
func (p *Point) inUse() []ActiveEntry {
	sc := p.story.scope
	var out []ActiveEntry
	add := func(owner string, entries []Entry) {
		for _, e := range entries {
			if !e.Disabled && p.knows(&e) {
				out = append(out, ActiveEntry{Owner: owner, Entry: e})
			}
		}
	}
	add(sc.scenario.ID, sc.scenario.Entries)
	for _, id := range sc.worldIDs {
		add(id, sc.worlds[id].Entries)
	}
	for _, id := range sc.cast {
		if c := sc.characters[id]; c != nil && (id == p.char || slices.Contains(p.state.present, id)) {
			add(id, c.Entries)
		}
	}
	add("session", p.state.entries)
	return out
}

// knows reports whether whoever the message being written is for knows e
// (Format 7.4).
func (p *Point) knows(e *Entry) bool {
	if p.char == "" {
		return e.KnownBy == nil || slices.Contains(e.KnownBy, narrator) || p.story.scope.scenario.Mode == "narrator"
	}
	return e.KnownBy == nil || slices.Contains(e.KnownBy, p.char)
}

// activates reports whether a, an entry in use, activates at p, before its
// group competes, and sets whether it is kept (Format 7.2).
func (p *Point) activates(a *ActiveEntry, r *rand.Rand) bool {
	e := &a.Entry
	// An entry recorded at a message of round t with sticky s stays active
	// through round t + s, and with cooldown c can't activate again through
	// round t + s + c.
	if t, ok := p.state.activated[a.Owner+"/"+e.ID]; ok {
		turn, end := int64(p.state.turn), int64(t)+e.Sticky
		switch {
		case e.Sticky > 0 && turn <= end:
			a.Kept = true
			return true
		case e.Once, e.Cooldown > 0 && turn <= end+e.Cooldown:
			return false
		}
	}
	depth := 4
	if e.ScanDepth != nil {
		depth = *e.ScanDepth
	}
	switch {
	case len(e.Keys) > 0:
		if !slices.ContainsFunc(e.Keys, func(k string) bool { return p.hit(k, depth) }) {
			return false
		}
	case e.Pull:
		return false
	}
	if e.When != "" {
		if ok, err := p.Eval(e.When, a.Owner, depth); err != nil || !ok {
			return false
		}
	}
	return e.Chance == nil || *e.Chance >= 100 || r.IntN(100) < *e.Chance
}

// compete keeps, of the active entries of each owner's group, one of the
// highest priority, chosen by weight among equals, where an entry sticky keeps
// wins against equals that aren't kept (Format 7.2).
func compete(active []ActiveEntry, r *rand.Rand) []ActiveEntry {
	groups := map[string][]int{} // each group's entries, by owner and group
	var order []string           // the groups, in the order of their first entries
	for i, a := range active {
		if a.Entry.Group == "" {
			continue
		}
		g := a.Owner + "/" + a.Entry.Group
		if groups[g] == nil {
			order = append(order, g)
		}
		groups[g] = append(groups[g], i)
	}
	lose := make([]bool, len(active))
	for _, g := range order {
		members := groups[g]
		top := 0
		for _, i := range members {
			top = max(top, priority(&active[i].Entry))
		}
		var best []int
		for _, i := range members {
			if priority(&active[i].Entry) == top {
				best = append(best, i)
			}
		}
		if kept := slices.DeleteFunc(slices.Clone(best), func(i int) bool { return !active[i].Kept }); len(kept) > 0 {
			best = kept
		}
		var total int64
		for _, i := range best {
			total += weight(&active[i].Entry)
		}
		winner := best[0]
		if len(best) > 1 {
			n := r.Int64N(total)
			for _, i := range best {
				if n -= weight(&active[i].Entry); n < 0 {
					winner = i
					break
				}
			}
		}
		for _, i := range members {
			lose[i] = i != winner
		}
	}
	var out []ActiveEntry
	for i, a := range active {
		if !lose[i] {
			out = append(out, a)
		}
	}
	return out
}

// priority returns e's priority, 50 when it has none.
func priority(e *Entry) int {
	if e.Priority == nil {
		return 50
	}
	return *e.Priority
}

// weight returns e's weight, 1 when it has none.
func weight(e *Entry) int64 {
	return int64(max(e.Weight, 1))
}
