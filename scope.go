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
	joiners    []string              // the cast's characters a session adds as joiners
	characters map[string]*Character // in use, by ID
	worlds     map[string]*World     // in use, by ID
	openings   map[string]bool
	// bindings are the variables in use: the scenario's, then its worlds',
	// then its characters' (Format 6.4), each in its owner's order.
	bindings []*binding
	byName   map[string]*binding
}

// binding is a variable in use, under its full name (Format 8.2).
type binding struct {
	name string // such as "state.bell" or "state.kai.nerve"
	v    *Variable
}

// newScope returns the scope of the bundle's scenario i, played in s when s
// isn't nil, or of no scenario when i is -1.
func newScope(b *Bundle, s *Session, i int) *scope {
	// Characters and worlds are the bundle's and the session's, the first of
	// each ID.
	characters, worlds := map[string]*Character{}, map[string]*World{}
	add := func(cs []Character, ws []World) {
		for j := range cs {
			if _, ok := characters[cs[j].ID]; !ok {
				characters[cs[j].ID] = &cs[j]
			}
		}
		for j := range ws {
			if _, ok := worlds[ws[j].ID]; !ok {
				worlds[ws[j].ID] = &ws[j]
			}
		}
	}
	add(b.Characters, b.Worlds)
	if s != nil {
		add(s.Header.Characters, s.Header.Worlds)
	}

	sc := &scope{
		index:      i,
		characters: map[string]*Character{},
		worlds:     map[string]*World{},
		openings:   map[string]bool{},
		byName:     map[string]*binding{},
	}
	var scenario *Scenario
	if i >= 0 {
		scenario = &b.Scenarios[i]
		sc.scenario = scenario
		sc.bind("state.", scenario.Variables)
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
		for _, c := range s.Header.Characters {
			if !slices.Contains(s.Header.Players, c.ID) {
				sc.joiners = append(sc.joiners, c.ID)
			}
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
		if w, ok := worlds[id]; ok && sc.worlds[id] == nil {
			sc.worlds[id] = w
			sc.bind("state."+id+".", w.Variables)
		}
	}
	for _, id := range inUse {
		if c, ok := characters[id]; ok && sc.characters[id] == nil {
			sc.characters[id] = c
			sc.bind("state."+id+".", c.Variables)
		}
	}
	return sc
}

// bind puts vars in use, each named prefix and its ID. A name already bound
// belongs to a variable with a duplicate ID, which is DUPLICATE_ID.
func (sc *scope) bind(prefix string, vars []Variable) {
	for i := range vars {
		b := &binding{name: prefix + vars[i].ID, v: &vars[i]}
		if _, ok := sc.byName[b.name]; !ok {
			sc.bindings = append(sc.bindings, b)
			sc.byName[b.name] = b
		}
	}
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

// variable returns the variable a change key names, and the key within it
// for a keyed variable. It reports whether the change key is valid (Format
// 8.5): a full name (Format 8.2), followed by a key the variable allows when
// it is keyed (Format 8.3).
func (sc *scope) variable(key string) (*binding, string, bool) {
	if b, ok := sc.byName[key]; ok {
		return b, "", !b.v.Of.keyed()
	}
	i := strings.LastIndexByte(key, '.')
	if i < 0 {
		return nil, "", false
	}
	b, ok := sc.byName[key[:i]]
	if !ok || !b.v.Of.keyed() || !sc.allows(b.v, key[i+1:]) {
		return nil, "", false
	}
	return b, key[i+1:], true
}

// keys returns the keys of a "cast" or list variable, in order; nil for a
// "*" map, whose keys appear as changes set them (Format 8.3).
func (sc *scope) keys(v *Variable) []string {
	if v.Of.Cast {
		return sc.cast
	}
	return v.Of.List
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
