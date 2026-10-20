package troupe

import (
	"regexp"
	"slices"
	"strings"
)

// scope is what a scenario has in use, as a session plays it or, without a
// session, as the scenario is written (Format 6.4).
type scope struct {
	all      *names // every character, world and location of the bundle and session
	scenario *Scenario
	index    int // the scenario's index in the bundle
	// cast is the cast's characters in cast order (Format 11.2). Without a
	// session, its seats are unfilled and left out (conformance README
	// section 2).
	cast []string
	// openingCast is the characters a "cast" variable has as keys in an
	// opening's set (Format 8.5): every character the cast members name,
	// seats marked player included. In a session, it is the cast, since a
	// root leaves out the keys of the seats no player fills (Format 11.1).
	openingCast []string
	// members are the cast's members, in cast order. Without a session,
	// they are the scenario's as written, seats included.
	members    []castMember
	players    []string              // the players' characters, in the order players lists them
	joiners    []string              // the cast's characters a session adds as joiners
	characters map[string]*Character // in use, by ID
	worlds     map[string]*World     // in use, by ID
	worldIDs   []string              // in use, in order: the scenario's, then the session's
	openings   map[string]bool
	// bindings are the variables in use: the scenario's, then its worlds',
	// then its characters' (Format 6.4), each in its owner's order.
	bindings []*binding
	byName   map[string]*binding
}

// castMember is a member of a cast.
type castMember struct {
	id   string      // its character; "" for a seat without one
	seat bool        // a member without a character, or one marked player (Format 6.1)
	of   *CastMember // the scenario's member it is; nil for a new seat or a joiner
}

// binding is a variable in use, under its full name (Format 8.2).
type binding struct {
	name  string // such as "state.bell" or "state.kai.nerve"
	owner string // the scenario's, world's or character's ID
	v     *Variable
	// values and keys are index(v.Values) and index(v.Of.List).
	values, keys map[string]int
}

// names indexes what references can name: the characters, worlds and
// locations of a bundle and its session, the first of each ID.
type names struct {
	characters map[string]*Character
	worlds     map[string]*World
	locations  map[string]*Location
}

// newNames returns the names of b, and of s when s isn't nil.
func newNames(b *Bundle, s *Session) *names {
	n := &names{characters: map[string]*Character{}, worlds: map[string]*World{}, locations: map[string]*Location{}}
	n.add(b.Characters, b.Worlds)
	if s != nil {
		n.add(s.Header.Characters, s.Header.Worlds)
	}
	return n
}

// add adds characters and worlds, and the worlds' locations.
func (n *names) add(characters []Character, worlds []World) {
	for i := range characters {
		if _, ok := n.characters[characters[i].ID]; !ok {
			n.characters[characters[i].ID] = &characters[i]
		}
	}
	for i := range worlds {
		if _, ok := n.worlds[worlds[i].ID]; !ok {
			n.worlds[worlds[i].ID] = &worlds[i]
		}
		for j, l := range worlds[i].Locations {
			if _, ok := n.locations[l.ID]; !ok {
				n.locations[l.ID] = &worlds[i].Locations[j]
			}
		}
	}
}

// scenarioIndex returns the index of b's scenario with the ID id, or -1.
func scenarioIndex(b *Bundle, id string) int {
	return slices.IndexFunc(b.Scenarios, func(sc Scenario) bool { return sc.ID == id })
}

// newScope returns the scope of the bundle's scenario i, played in s when s
// isn't nil.
func newScope(b *Bundle, s *Session, i int) *scope {
	n := newNames(b, s)
	scenario := &b.Scenarios[i]
	sc := &scope{
		all:        n,
		scenario:   scenario,
		index:      i,
		characters: map[string]*Character{},
		worlds:     map[string]*World{},
		openings:   map[string]bool{},
		byName:     map[string]*binding{},
	}
	sc.bind(scenario.ID, "state.", scenario.Variables)
	for _, o := range scenario.Openings {
		sc.openings[o.ID] = true
	}

	members := scenario.Cast
	if members == nil {
		// One player seat, then every bundle character (Format 6.1).
		members = []CastMember{{}}
		for _, c := range b.Characters {
			members = append(members, CastMember{Character: c.ID})
		}
	}

	var inUse []string
	if s == nil {
		for i, m := range members {
			sc.members = append(sc.members, castMember{id: m.Character, seat: m.Character == "" || m.Player, of: &members[i]})
			if m.Character != "" {
				inUse = append(inUse, m.Character)
				if !m.Player {
					sc.cast = append(sc.cast, m.Character)
				}
			}
		}
	} else {
		sc.members, sc.joiners = sessionCast(members, &s.Header)
		for _, m := range sc.members {
			sc.cast = append(sc.cast, m.id)
		}
		sc.players = s.Header.Players
		inUse = sc.cast
	}
	sc.openingCast = inUse

	var worldIDs []string
	if scenario.Worlds != nil {
		// A copy, since the session's are appended, and the bundle may be
		// shared.
		worldIDs = slices.Clone(*scenario.Worlds)
	} else {
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
		if w, ok := n.worlds[id]; ok && sc.worlds[id] == nil {
			sc.worlds[id] = w
			sc.worldIDs = append(sc.worldIDs, id)
			sc.bind(id, "state."+id+".", w.Variables)
		}
	}
	for _, id := range inUse {
		if c, ok := n.characters[id]; ok && sc.characters[id] == nil {
			sc.characters[id] = c
			sc.bind(id, "state."+id+".", c.Variables)
		}
	}
	return sc
}

// bind puts owner's vars in use, each named prefix and its ID. A name already
// bound belongs to a variable with a duplicate ID, which is DUPLICATE_ID.
func (sc *scope) bind(owner, prefix string, vars []Variable) {
	for i := range vars {
		v := &vars[i]
		b := &binding{name: prefix + v.ID, owner: owner, v: v, values: index(v.Values), keys: index(v.Of.List)}
		if _, ok := sc.byName[b.name]; !ok {
			sc.bindings = append(sc.bindings, b)
			sc.byName[b.name] = b
		}
	}
}

// sessionCast returns a session's cast (Format 11.2): the scenario cast with
// its seats filled, then the new seats, then the joiners: the header
// characters that aren't players, which it also returns.
func sessionCast(members []CastMember, h *SessionHeader) (cast []castMember, joiners []string) {
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
	for i, m := range members {
		switch {
		case m.Character == "":
			if len(fill) > 0 {
				cast = append(cast, castMember{id: fill[0], seat: true, of: &members[i]})
				fill = fill[1:]
			}
		case !m.Player || slices.Contains(h.Players, m.Character):
			cast = append(cast, castMember{id: m.Character, seat: m.Player, of: &members[i]})
		}
	}
	for _, id := range fill {
		cast = append(cast, castMember{id: id, seat: true})
	}
	for _, c := range h.Characters {
		if !slices.Contains(h.Players, c.ID) {
			joiners = append(joiners, c.ID)
			cast = append(cast, castMember{id: c.ID})
		}
	}
	return cast, joiners
}

// variable returns the variable a change key names, and the key within it
// for a keyed variable. It reports whether the change key is valid (Format
// 8.5): a full name (Format 8.2), followed by a key the variable allows when
// it is keyed (Format 8.3).
func (sc *scope) variable(key string) (*binding, string, bool) {
	return sc.lookup(key, sc.cast)
}

// openingVariable is variable for a key of an opening's set, where a "cast"
// variable has the keys of openingCast (Format 8.5).
func (sc *scope) openingVariable(key string) (*binding, string, bool) {
	return sc.lookup(key, sc.openingCast)
}

// lookup is variable, with cast as a "cast" variable's keys.
func (sc *scope) lookup(key string, cast []string) (*binding, string, bool) {
	if b, ok := sc.byName[key]; ok {
		return b, "", !b.v.Of.keyed()
	}
	i := strings.LastIndexByte(key, '.')
	if i < 0 {
		return nil, "", false
	}
	b, ok := sc.byName[key[:i]]
	if !ok || !b.v.Of.keyed() || !allows(b, key[i+1:], cast) {
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

// allows reports whether a keyed variable has the key k, with cast as a
// "cast" variable's keys (Format 8.3).
func allows(b *binding, k string, cast []string) bool {
	switch {
	case b.v.Of.Cast:
		return slices.Contains(cast, k)
	case b.v.Of.Any:
		return isKey(k)
	default:
		_, ok := b.keys[k]
		return ok
	}
}

// entry reports whether an entry of a scenario, world or character in use has
// the ID id (Format 6.4). Session entries are the caller's to find.
func (sc *scope) entry(owner, id string) bool {
	var entries []Entry
	switch {
	case sc.scenario.ID == owner:
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

// declared resolves a macro's state or own path to the variable it names
// (Format 9): its full name, its declaration, and the path's segments below
// it. It reports false when the path names no declared variable, as Format
// 10.2 counts declarations: state against scenario's variables, when it isn't
// nil, and those of every character and world; own against own.
func (n *names) declared(scenario *Scenario, own ownVars, isOwn bool, path []string) (string, *Variable, []string, bool) {
	owner := own
	switch {
	case isOwn:
	case scenario != nil && variableOf(scenario.Variables, path[0]) != nil:
		owner = scenario.own()
	case len(path) < 2:
		return "", nil, nil, false
	default:
		// A character's or world's variable: what own means in its own text.
		owner = n.ownOf(nil, path[0])
		path = path[1:]
	}
	v := variableOf(owner.list(), path[0])
	if v == nil {
		return "", nil, nil, false
	}
	return owner.prefix + path[0], v, path[1:], true
}

// ownVars are the variables own means in an owner's texts and conditions,
// and what their full names start with (Format 8.2). vars is the owner's
// field, so two are equal only for the same owner, even when owners share an
// ID. A session entry's are the zero ownVars, since it has none.
type ownVars struct {
	vars   *[]Variable
	prefix string
}

func (o ownVars) list() []Variable {
	if o.vars == nil {
		return nil
	}
	return *o.vars
}

func (sc *Scenario) own() ownVars { return ownVars{&sc.Variables, "state."} }
func (c *Character) own() ownVars { return ownVars{&c.Variables, "state." + c.ID + "."} }
func (w *World) own() ownVars     { return ownVars{&w.Variables, "state." + w.ID + "."} }

// ownOf returns the ownVars of the owner with the ID id: scenario, when it
// isn't nil, or the first character or world; none for any other ID, such as
// "session". Only without DUPLICATE_ID does an ID name one owner, so a caller
// holding the owner takes its own instead.
func (n *names) ownOf(scenario *Scenario, id string) ownVars {
	switch {
	case scenario != nil && id == scenario.ID:
		return scenario.own()
	case n.characters[id] != nil:
		return n.characters[id].own()
	case n.worlds[id] != nil:
		return n.worlds[id].own()
	}
	return ownVars{}
}

// variableOf returns the variable of vars with the ID id, or nil.
func variableOf(vars []Variable, id string) *Variable {
	i := slices.IndexFunc(vars, func(v Variable) bool { return v.ID == id })
	if i < 0 {
		return nil
	}
	return &vars[i]
}
