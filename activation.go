package troupe

import (
	"math/rand/v2"
	"slices"
)

// ActiveEntry is an entry active for the message being written, or one whose
// retrieval would activate it (Format 7.2).
type ActiveEntry struct {
	// Owner is the entry's owner: a character, world or scenario ID, or
	// "session" for a session entry. Render and Eval take it as the owner.
	Owner string
	Entry Entry
	// Kept reports whether the entry's sticky keeps it active from a message
	// that records it, so the message being written doesn't record it in
	// activated again (Format 11.4).
	Kept bool
}

// Ref returns the entry's reference: its owner, "/" and its ID (Format 2.4).
func (a *ActiveEntry) Ref() string {
	return a.Owner + "/" + a.Entry.ID
}

// Priority returns the entry's priority, 50 when it has none (Format 7).
func (a *ActiveEntry) Priority() int {
	if a.Entry.Priority == nil {
		return 50
	}
	return *a.Entry.Priority
}

// group names the entry's group among every owner's.
func (a *ActiveEntry) group() string {
	return a.Owner + "/" + a.Entry.Group
}

// Active returns the entries active for the message being written at p
// (Format 7.2), in the order of their owners, the scenario, its worlds, the
// characters of the cast and the session, and each owner's in its order. The
// entries in use are the scenario's, its worlds', those of the characters
// present or the message is written for, and the session's; of these, those
// whoever the message is for knows (Format 7.4).
//
// An entry with keywords activates when one matches its scanned text, a
// retrievable one when retrieved names its reference, one with both by either,
// and one with neither always, while its when holds. retrieved holds the Ref
// of each entry the caller retrieved, of those Retrievable gives; any other
// reference changes nothing. The rounds of the messages that record an entry
// in activated time it: sticky keeps it active, and once and cooldown keep it
// from activating again. Of the entries that would activate, group keeps one
// of each owner's group, of the highest priority, chosen by weight among
// equals, where an entry sticky keeps wins against equals that aren't kept,
// and then chance lets each through, so a chosen entry that fails it leaves
// its group empty. r draws for weight and chance, so activation is
// reproducible from the caller's source.
//
// Which active entries reach the model is the caller's to choose, by their
// Priority, and where each goes by its position or depth (Format 7.3). The
// message being written records the Ref of those that reach it in activated,
// apart from those Kept; one whose text renders as nothing or only whitespace
// reaches nothing (Format 11.4).
func (p *Point) Active(retrieved []string, r *rand.Rand) []ActiveEntry {
	wanted := make(map[string]bool, len(retrieved))
	for _, ref := range retrieved {
		wanted[ref] = true
	}
	var active []ActiveEntry
	for _, a := range p.inUse() {
		if p.activates(&a, wanted[a.Ref()]) {
			active = append(active, a)
		}
	}
	return slices.DeleteFunc(compete(active, r), func(a ActiveEntry) bool {
		c := a.Entry.Chance
		return !a.Kept && c != nil && *c < 100 && r.IntN(100) >= *c
	})
}

// Retrievable returns the retrievable entries that retrieving would activate
// at p, in Active's order: those that activate when retrieved and not
// otherwise, and that can win their group, since no entry of it active without
// retrieval has a higher priority, or an equal one and is kept (Format 7.2).
// chance is drawn only once they are retrieved. The caller retrieves those
// that are relevant, by their title or text, and passes each one's Ref to
// Active.
func (p *Point) Retrievable() []ActiveEntry {
	var out []ActiveEntry
	top := map[string]int{} // the highest rank of each group's entries active without retrieval
	for _, a := range p.inUse() {
		switch {
		case p.activates(&a, false):
			if a.Entry.Group != "" {
				top[a.group()] = max(top[a.group()], rank(&a))
			}
		case a.Entry.Retrievable && p.activates(&a, true):
			out = append(out, a)
		}
	}
	return slices.DeleteFunc(out, func(a ActiveEntry) bool {
		return a.Entry.Group != "" && top[a.group()] > rank(&a)
	})
}

// inUse returns the entries in use at p that whoever the message being
// written is for knows, and that aren't disabled (Format 6.4, 7.1, 7.4).
func (p *Point) inUse() []ActiveEntry {
	sc := p.play.scope
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
		return e.KnownBy == nil || slices.Contains(e.KnownBy, narrator) || p.play.scope.scenario.Mode == "narrator"
	}
	return e.KnownBy == nil || slices.Contains(e.KnownBy, p.char)
}

// activates reports whether a, an entry in use, activates at p, retrieved or
// not, before its group competes and its chance is drawn, and sets whether it
// is kept (Format 7.2).
func (p *Point) activates(a *ActiveEntry, retrieved bool) bool {
	e := &a.Entry
	// An entry recorded at a message of round t with sticky s stays active
	// through round t + s, and with cooldown c can't activate again through
	// round t + s + c.
	if t, ok := p.state.activated[a.Ref()]; ok {
		round, end := int64(p.state.round), int64(t)+e.Sticky
		switch {
		case e.Sticky > 0 && round <= end:
			a.Kept = true
			return true
		case e.Once, e.Cooldown > 0 && round <= end+e.Cooldown:
			return false
		}
	}
	depth := 4
	if e.ScanDepth != nil {
		depth = *e.ScanDepth
	}
	switch {
	case e.Retrievable && retrieved:
	case slices.ContainsFunc(e.Keywords, func(k string) bool { return p.hit(k, depth) }):
	case len(e.Keywords) > 0 || e.Retrievable:
		return false
	}
	if e.When != "" {
		if ok, err := p.Eval(e.When, a.Owner, depth); err != nil || !ok {
			return false
		}
	}
	return true
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
		g := a.group()
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
			top = max(top, rank(&active[i]))
		}
		var best []int
		for _, i := range members {
			if rank(&active[i]) == top {
				best = append(best, i)
			}
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

// rank orders the entries of a group before weight chooses among equals: by
// priority, and an entry kept above one that isn't.
func rank(a *ActiveEntry) int {
	r := 2 * a.Priority()
	if a.Kept {
		r++
	}
	return r
}

// weight returns e's weight, 1 when it has none.
func weight(e *Entry) int64 {
	return int64(max(e.Weight, 1))
}
