package troupe

import (
	"regexp"
	"slices"
	"strings"
)

// scope is what a scenario has in use, as a session plays it or, without a
// session, as the scenario is written (Format 6.4).
type scope struct {
	scenario *Scenario // nil when the session names none
	index    int       // the scenario's index in the bundle
	// cast is the cast's characters in cast order (Format 11.2). Without a
	// session, its seats are unfilled and left out (conformance README
	// section 2).
	cast       []string
	characters map[string]*Character // in use, by ID
	worlds     map[string]*World     // in use, by ID
	variables  map[string]*Variable  // the scenario's, by ID
	openings   map[string]bool
}

// newScope returns the scope of the bundle's scenario i, played in s when s
// isn't nil. Characters and worlds come from lookup tables of the bundle's and
// the session's, the first of each ID.
func newScope(b *Bundle, s *Session, i int, characters map[string]*Character, worlds map[string]*World) *scope {
	sc := &scope{
		index:      i,
		characters: map[string]*Character{},
		worlds:     map[string]*World{},
		variables:  map[string]*Variable{},
		openings:   map[string]bool{},
	}
	var scenario *Scenario
	if i >= 0 {
		scenario = &b.Scenarios[i]
		sc.scenario = scenario
		for j := range scenario.Variables {
			sc.variables[scenario.Variables[j].ID] = &scenario.Variables[j]
		}
		for _, o := range scenario.Openings {
			sc.openings[o.ID] = true
		}
	}

	var members []CastMember
	switch {
	case scenario == nil:
	case scenario.Cast != nil:
		members = scenario.Cast
	default:
		// One player seat, then every bundle character (Format 6.1).
		members = []CastMember{{}}
		for _, c := range b.Characters {
			members = append(members, CastMember{Character: c.ID})
		}
	}

	var inUse []string
	if s == nil {
		for _, m := range members {
			if m.Character != "" {
				inUse = append(inUse, m.Character)
				if !m.Player {
					sc.cast = append(sc.cast, m.Character)
				}
			}
		}
	} else {
		sc.cast = sessionCast(members, &s.Header)
		inUse = sc.cast
	}
	for _, id := range inUse {
		if c, ok := characters[id]; ok {
			sc.characters[id] = c
		}
	}

	var worldIDs []string
	switch {
	case scenario == nil:
	case scenario.Worlds != nil:
		worldIDs = *scenario.Worlds
	default:
		for _, w := range b.Worlds {
			worldIDs = append(worldIDs, w.ID)
		}
	}
	if s != nil {
		for _, w := range s.Header.Worlds {
			worldIDs = append(worldIDs, w.ID)
		}
	}
	for _, id := range worldIDs {
		if w, ok := worlds[id]; ok {
			sc.worlds[id] = w
		}
	}
	return sc
}

// sessionCast returns a session's cast (Format 11.2): the scenario cast with
// its seats filled, then the new seats, then the joiners.
func sessionCast(members []CastMember, h *SessionHeader) []string {
	isHeader := map[string]bool{}
	for _, c := range h.Characters {
		isHeader[c.ID] = true
	}
	// The header characters players play fill the seats without a character,
	// in the order of players.
	var fill []string
	for _, p := range h.Players {
		if isHeader[p] {
			fill = append(fill, p)
		}
	}
	var cast []string
	for _, m := range members {
		switch {
		case m.Character == "":
			if len(fill) > 0 {
				cast = append(cast, fill[0])
				fill = fill[1:]
			}
		case !m.Player || slices.Contains(h.Players, m.Character):
			cast = append(cast, m.Character)
		}
	}
	cast = append(cast, fill...)
	for _, c := range h.Characters {
		if !slices.Contains(h.Players, c.ID) {
			cast = append(cast, c.ID)
		}
	}
	return cast
}

// variable returns the variable a change key names, and whether the key is
// valid (Format 8.5): a full name (Format 8.2), followed by a key the variable
// allows when it is keyed (Format 8.3).
func (sc *scope) variable(key string) (*Variable, bool) {
	segs := strings.Split(key, ".")
	if len(segs) < 2 || segs[0] != "state" {
		return nil, false
	}
	segs = segs[1:]
	v, ok := sc.variables[segs[0]]
	if ok {
		segs = segs[1:]
	} else {
		if len(segs) < 2 {
			return nil, false
		}
		if v = sc.ownVariable(segs[0], segs[1]); v == nil {
			return nil, false
		}
		segs = segs[2:]
	}
	if !v.Of.keyed() {
		return v, len(segs) == 0
	}
	return v, len(segs) == 1 && sc.allows(v, segs[0])
}

// ownVariable returns the variable id of the character or world owner, when
// both are in use.
func (sc *scope) ownVariable(owner, id string) *Variable {
	var vars []Variable
	if c, ok := sc.characters[owner]; ok {
		vars = c.Variables
	} else if w, ok := sc.worlds[owner]; ok {
		vars = w.Variables
	}
	for i := range vars {
		if vars[i].ID == id {
			return &vars[i]
		}
	}
	return nil
}

// allows reports whether a keyed variable has the key k (Format 8.3).
func (sc *scope) allows(v *Variable, k string) bool {
	switch {
	case v.Of.Cast:
		return slices.Contains(sc.cast, k)
	case v.Of.Any:
		return isKey(k)
	default:
		return slices.Contains(v.Of.List, k)
	}
}

// entry reports whether an entry of a scenario, world or character in use has
// the ID id (Format 6.4). Session entries are the caller's to find.
func (sc *scope) entry(owner, id string) bool {
	var entries []Entry
	switch {
	case sc.scenario != nil && sc.scenario.ID == owner:
		entries = sc.scenario.Entries
	case sc.characters[owner] != nil:
		entries = sc.characters[owner].Entries
	case sc.worlds[owner] != nil:
		entries = sc.worlds[owner].Entries
	}
	return slices.ContainsFunc(entries, func(e Entry) bool { return e.ID == id })
}

func (k Keys) keyed() bool {
	return k.Cast || k.Any || k.List != nil
}

var authorID = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// celReserved are the words no author ID can be (Format 2.3).
var celReserved = []string{
	"as", "break", "const", "continue", "else", "false", "for", "function", "if", "import",
	"in", "let", "loop", "namespace", "null", "package", "return", "true", "var", "void", "while",
}

// isKey reports whether k can be a key of a "*" map: an author ID other than
// char and user (Format 8.3).
func isKey(k string) bool {
	return authorID.MatchString(k) && !slices.Contains(celReserved, k) && k != "char" && k != "user"
}
