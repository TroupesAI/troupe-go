package troupe

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"slices"
	"strconv"
	"strings"
)

// Check runs the checks of Format 15.3 that reading leaves: IDs, references,
// variable keys and values, messages and players. b is a bundle that read
// without PARSE, and s, when not nil, a session played with it.
//
// The bundle is checked once for each of its scenarios, as written and played
// without players, whether or not a session comes with it. A session is then
// checked against its scenario (conformance README section 2). Diagnostics are
// sorted as reading's are, and their File is empty.
func Check(b *Bundle, s *Session) []Diagnostic {
	c := &checker{
		b:           b,
		characters:  map[string]*Character{},
		worlds:      map[string]*World{},
		locations:   map[string]bool{},
		owners:      map[string]bool{},
		locationIDs: map[string]bool{},
	}
	c.add(b.Characters, b.Worlds)
	c.bundleIDs()
	c.bundleReferences()
	c.initials(inBundle, b.Characters, b.Worlds, b.Scenarios)
	for i := range b.Scenarios {
		c.openings(newScope(b, nil, i, c.characters, c.worlds))
	}
	if s != nil {
		c.s = s
		c.add(s.Header.Characters, s.Header.Worlds)
		scenario := slices.IndexFunc(b.Scenarios, func(sc Scenario) bool { return sc.ID == s.Header.Scenario })
		c.sessionIDs()
		c.sessionReferences(scenario)
		c.initials(inHeader, s.Header.Characters, s.Header.Worlds, nil)
		c.messages(newScope(b, s, scenario, c.characters, c.worlds))
	}
	return sortDiagnostics(c.diags)
}

type checker struct {
	b *Bundle
	s *Session // nil until the session's checks
	// characters and worlds are the first of each ID, and locations every
	// location's ID: the bundle's, and once its checks begin, the session's.
	characters map[string]*Character
	worlds     map[string]*World
	locations  map[string]bool
	// owners and locationIDs are the IDs seen, for DUPLICATE_ID.
	owners      map[string]bool
	locationIDs map[string]bool
	diags       []Diagnostic
}

// add adds characters and worlds, and the worlds' locations, to those
// references can name.
func (c *checker) add(characters []Character, worlds []World) {
	for i := range characters {
		if _, ok := c.characters[characters[i].ID]; !ok {
			c.characters[characters[i].ID] = &characters[i]
		}
	}
	for i := range worlds {
		if _, ok := c.worlds[worlds[i].ID]; !ok {
			c.worlds[worlds[i].ID] = &worlds[i]
		}
		for _, l := range worlds[i].Locations {
			c.locations[l.ID] = true
		}
	}
}

// place is a document the diagnostics are in: the bundle, a session's header
// or one of its messages.
type place struct {
	line    int
	message string
}

var (
	inBundle = place{}
	inHeader = place{line: 1}
)

func inMessage(i int, m *Message) place {
	at := place{line: i + 2}
	if messageID.MatchString(m.ID) {
		at.message = m.ID
	}
	return at
}

func (c *checker) report(code Code, at place, p jsontext.Pointer) {
	c.diags = append(c.diags, Diagnostic{Code: code, Line: at.line, Message: at.message, Pointer: string(p)})
}

// ptr appends tokens, each a string or an int, to p.
func ptr(p jsontext.Pointer, tokens ...any) jsontext.Pointer {
	for _, t := range tokens {
		switch t := t.(type) {
		case string:
			p = p.AppendToken(t)
		case int:
			p = p.AppendToken(strconv.Itoa(t))
		}
	}
	return p
}

// bundleIDs checks that the bundle's IDs are unique as Format 2.3 requires.
// A duplicate is reported at each ID after the first, in the order of
// conformance README section 3.4.
func (c *checker) bundleIDs() {
	b := c.b
	for i, ch := range b.Characters {
		c.unique(c.owners, ch.ID, inBundle, ptr("", "characters", i, "id"))
	}
	for i, w := range b.Worlds {
		c.unique(c.owners, w.ID, inBundle, ptr("", "worlds", i, "id"))
	}
	for i, sc := range b.Scenarios {
		c.unique(c.owners, sc.ID, inBundle, ptr("", "scenarios", i, "id"))
	}
	for i, w := range b.Worlds {
		for j, l := range w.Locations {
			c.unique(c.locationIDs, l.ID, inBundle, ptr("", "worlds", i, "locations", j, "id"))
		}
	}
	for i, ch := range b.Characters {
		c.ownIDs(inBundle, ptr("", "characters", i), ch.Entries, ch.Variables)
	}
	for i, w := range b.Worlds {
		c.ownIDs(inBundle, ptr("", "worlds", i), w.Entries, w.Variables)
	}
	for i, sc := range b.Scenarios {
		p := ptr("", "scenarios", i)
		c.ownIDs(inBundle, p, sc.Entries, sc.Variables)
		for j, v := range sc.Variables {
			// A scenario variable's full name would read as an owner's.
			if c.characters[v.ID] != nil || c.worlds[v.ID] != nil {
				c.report(CodeDuplicateID, inBundle, ptr(p, "variables", j, "id"))
			}
		}
		cast, worlds, openings := map[string]bool{}, map[string]bool{}, map[string]bool{}
		for j, m := range sc.Cast {
			c.unique(cast, m.Character, inBundle, ptr(p, "cast", j, "character"))
		}
		if sc.Worlds != nil {
			for j, w := range *sc.Worlds {
				c.unique(worlds, w, inBundle, ptr(p, "worlds", j))
			}
		}
		for j, o := range sc.Openings {
			c.unique(openings, o.ID, inBundle, ptr(p, "openings", j, "id"))
		}
	}
}

// sessionIDs checks the session's IDs, after the bundle's: its characters and
// worlds share the bundle's namespace, and its locations the bundle's
// locations' (Format 2.3).
func (c *checker) sessionIDs() {
	h := &c.s.Header
	for i, ch := range h.Characters {
		c.unique(c.owners, ch.ID, inHeader, ptr("", "characters", i, "id"))
		c.scenarioVariable(ch.ID, ptr("", "characters", i, "id"))
	}
	for i, w := range h.Worlds {
		c.unique(c.owners, w.ID, inHeader, ptr("", "worlds", i, "id"))
		c.scenarioVariable(w.ID, ptr("", "worlds", i, "id"))
	}
	for i, w := range h.Worlds {
		for j, l := range w.Locations {
			c.unique(c.locationIDs, l.ID, inHeader, ptr("", "worlds", i, "locations", j, "id"))
		}
	}
	for i, ch := range h.Characters {
		c.ownIDs(inHeader, ptr("", "characters", i), ch.Entries, ch.Variables)
	}
	for i, w := range h.Worlds {
		c.ownIDs(inHeader, ptr("", "worlds", i), w.Entries, w.Variables)
	}
	messages := map[string]bool{}
	for i := range c.s.Messages {
		m := &c.s.Messages[i]
		c.unique(messages, m.ID, inMessage(i, m), "/id")
		c.ownIDs(inMessage(i, m), "", m.Entries, nil)
	}
}

// scenarioVariable reports a session's character or world, at p, that takes a
// scenario variable's ID (Format 2.3). It is the session's to report, since
// the bundle is valid without it.
func (c *checker) scenarioVariable(id string, p jsontext.Pointer) {
	for _, sc := range c.b.Scenarios {
		if slices.ContainsFunc(sc.Variables, func(v Variable) bool { return v.ID == id }) {
			c.report(CodeDuplicateID, inHeader, p)
			return
		}
	}
}

// unique records id as seen, reporting it when it was already. A missing ID
// is the schema's to report.
func (c *checker) unique(seen map[string]bool, id string, at place, p jsontext.Pointer) {
	if id == "" {
		return
	}
	if seen[id] {
		c.report(CodeDuplicateID, at, p)
	}
	seen[id] = true
}

// ownIDs checks the IDs of one owner's entries, and of its variables. An
// entry and a variable may share an ID.
func (c *checker) ownIDs(at place, p jsontext.Pointer, entries []Entry, variables []Variable) {
	seen := map[string]bool{}
	for i, e := range entries {
		c.unique(seen, e.ID, at, ptr(p, "entries", i, "id"))
	}
	seen = map[string]bool{}
	for i, v := range variables {
		c.unique(seen, v.ID, at, ptr(p, "variables", i, "id"))
	}
}

// bundleReferences checks that the bundle's references name what they refer
// to, other than those that depend on a scenario (Format 15.3).
func (c *checker) bundleReferences() {
	b := c.b
	for i := range b.Characters {
		c.character(inBundle, ptr("", "characters", i), &b.Characters[i])
	}
	for i, w := range b.Worlds {
		c.entries(inBundle, ptr("", "worlds", i), w.Entries)
	}
	for i, sc := range b.Scenarios {
		p := ptr("", "scenarios", i)
		for j, m := range sc.Cast {
			if m.Character != "" && c.characters[m.Character] == nil {
				c.report(CodeUnknownCharacter, inBundle, ptr(p, "cast", j, "character"))
			}
		}
		if sc.Worlds != nil {
			for j, w := range *sc.Worlds {
				if c.worlds[w] == nil {
					c.report(CodeUnknownWorld, inBundle, ptr(p, "worlds", j))
				}
			}
		}
		c.entries(inBundle, p, sc.Entries)
		for j, o := range sc.Openings {
			c.parts(inBundle, ptr(p, "openings", j, "content"), o.Content)
		}
	}
	c.parents(inBundle)
}

// sessionReferences checks that the session's references name what they refer
// to, in the bundle or the session, other than those that depend on its
// scenario, the bundle's scenario at index scenario or -1 for none.
func (c *checker) sessionReferences(scenario int) {
	b, s, h := c.b, c.s, &c.s.Header
	for i := range h.Characters {
		c.character(inHeader, ptr("", "characters", i), &h.Characters[i])
	}
	for i, w := range h.Worlds {
		c.entries(inHeader, ptr("", "worlds", i), w.Entries)
	}
	c.parents(inHeader)
	if scenario < 0 && h.Scenario != "" {
		c.report(CodeUnknownScenario, inHeader, "/scenario")
	}
	// A person plays a header character or a cast member (Format 11.2).
	for j, p := range h.Players {
		player := slices.ContainsFunc(h.Characters, func(ch Character) bool { return ch.ID == p })
		if scenario >= 0 {
			if cast := b.Scenarios[scenario].Cast; cast != nil {
				player = player || slices.ContainsFunc(cast, func(m CastMember) bool { return m.Character == p })
			} else {
				player = player || slices.ContainsFunc(b.Characters, func(ch Character) bool { return ch.ID == p })
			}
		}
		if !player {
			c.report(CodeInvalidPlayer, inHeader, ptr("", "players", j))
		}
	}
	ids := map[string]bool{}
	for j := range s.Messages {
		m := &s.Messages[j]
		at := inMessage(j, m)
		if m.Parent != "" && !ids[m.Parent] {
			c.report(CodeUnknownMessage, at, "/parent")
		}
		ids[m.ID] = true
		if m.Char != "" && c.characters[m.Char] == nil {
			c.report(CodeUnknownCharacter, at, "/char")
		}
		c.parts(at, "/content", m.Content)
		c.entries(at, "", m.Entries)
	}
	if h.Head != "" && !ids[h.Head] {
		c.report(CodeUnknownMessage, inHeader, "/head")
	}
}

// character checks a character's example speakers and entries.
func (c *checker) character(at place, p jsontext.Pointer, ch *Character) {
	for i, exchange := range ch.Examples {
		for j, l := range exchange {
			if l.Speaker != "" && l.Speaker != "char" && l.Speaker != "user" && c.characters[l.Speaker] == nil {
				c.report(CodeUnknownCharacter, at, ptr(p, "examples", i, j, "speaker"))
			}
		}
	}
	c.entries(at, p, ch.Entries)
}

// entries checks the knownBy of the entries of the owner at p.
func (c *checker) entries(at place, p jsontext.Pointer, entries []Entry) {
	for i, e := range entries {
		c.knownBy(at, ptr(p, "entries", i, "knownBy"), e.KnownBy)
	}
}

// parts checks the speakers and knownBy of the parts at p.
func (c *checker) parts(at place, p jsontext.Pointer, parts []Part) {
	for i, part := range parts {
		if part.Speaker != "" && c.characters[part.Speaker] == nil {
			c.report(CodeUnknownCharacter, at, ptr(p, i, "speaker"))
		}
		c.knownBy(at, ptr(p, i, "knownBy"), part.KnownBy)
	}
}

func (c *checker) knownBy(at place, p jsontext.Pointer, ids []string) {
	for i, id := range ids {
		if id != "narrator" && c.characters[id] == nil {
			c.report(CodeUnknownCharacter, at, ptr(p, i))
		}
	}
}

// parents checks that every location of the document at, the bundle or a
// session's header, has a parent that names a location, without cycles. A
// cycle is reported at the parent of each location on it. A session's
// locations may name the bundle's, but not the other way around.
func (c *checker) parents(at place) {
	type location struct {
		at     place
		p      jsontext.Pointer // its parent's
		parent string
	}
	var locs []location
	index := map[string]int{} // the first location of each ID
	add := func(at place, worlds []World) {
		for i, w := range worlds {
			for j, l := range w.Locations {
				if _, ok := index[l.ID]; !ok {
					index[l.ID] = len(locs)
				}
				locs = append(locs, location{at, ptr("", "worlds", i, "locations", j, "parent"), l.Parent})
			}
		}
	}
	add(inBundle, c.b.Worlds)
	if at == inHeader {
		add(inHeader, c.s.Header.Worlds)
	}
	report := func(l location) {
		if l.at == at {
			c.report(CodeUnknownLocation, l.at, l.p)
		}
	}
	// Each location has at most one parent, so a walk from a location not yet
	// seen ends at one without a parent, or at one seen before: on this walk,
	// which closes a cycle, or on an earlier one.
	const (
		unseen = iota
		walking
		done
	)
	state := make([]int, len(locs))
	for start := range locs {
		var path []int
		i := start
		for i >= 0 && state[i] == unseen {
			state[i] = walking
			path = append(path, i)
			next, ok := index[locs[i].parent]
			switch {
			case locs[i].parent == "":
				i = -1
			case !ok:
				report(locs[i])
				i = -1
			default:
				i = next
			}
		}
		if i >= 0 && state[i] == walking {
			for _, j := range path[slices.Index(path, i):] {
				report(locs[j])
			}
		}
		for _, j := range path {
			state[j] = done
		}
	}
}

// initials checks the initial values of the variables of the characters,
// worlds and scenarios of the document at (Format 8.1, 8.3, 8.5).
func (c *checker) initials(at place, characters []Character, worlds []World, scenarios []Scenario) {
	check := func(p jsontext.Pointer, vars []Variable) {
		for i := range vars {
			c.initial(at, ptr(p, "variables", i), &vars[i])
		}
	}
	for i, ch := range characters {
		check(ptr("", "characters", i), ch.Variables)
	}
	for i, w := range worlds {
		check(ptr("", "worlds", i), w.Variables)
	}
	for i, sc := range scenarios {
		check(ptr("", "scenarios", i), sc.Variables)
	}
}

// initial checks one variable's initial value. A keyed variable's may be an
// object of values by key, where "*" gives every other key's (Format 8.3).
// Its members for keys the variable doesn't have are ignored, except that one
// of a "cast" variable that names no character is UNKNOWN_CHARACTER. Which
// characters are in the cast depends on the scenario, so a member that names
// any character is checked.
func (c *checker) initial(at place, p jsontext.Pointer, v *Variable) {
	if v.Initial == nil {
		return
	}
	p = ptr(p, "initial")
	if !v.Of.keyed() || v.Initial.Kind() != '{' {
		if !validValue(v, v.Initial) {
			c.report(CodeInvalidValue, at, p)
		}
		return
	}
	var members map[string]jsontext.Value
	if err := json.Unmarshal(v.Initial, &members); err != nil {
		panic(err) // an object that read without PARSE
	}
	for k, val := range members {
		switch {
		case k == "*":
		case v.Of.Cast && c.characters[k] == nil:
			c.report(CodeUnknownCharacter, at, ptr(p, k))
			continue
		case v.Of.List != nil && !slices.Contains(v.Of.List, k):
			continue
		}
		if !validValue(v, val) {
			c.report(CodeInvalidValue, at, ptr(p, k))
		}
	}
}

// openings checks the keys and values of the openings of sc's scenario.
func (c *checker) openings(sc *scope) {
	for j, o := range sc.scenario.Openings {
		c.set(sc, inBundle, ptr("", "scenarios", sc.index, "openings", j, "set"), o.Set)
	}
}

// messages runs the checks of a session's messages that depend on what its
// scenario has in use, sc: the keys and values of changes, the openings roots
// name, and the entries messages record as activated.
func (c *checker) messages(sc *scope) {
	msgs := c.s.Messages
	for i := range msgs {
		m := &msgs[i]
		if m.Parent == "" && m.Opening != "" && !sc.openings[m.Opening] {
			c.report(CodeUnknownOpening, inMessage(i, m), "/opening")
		}
		for j, ch := range m.Changes {
			c.set(sc, inMessage(i, m), ptr("", "changes", j, "set"), ch.Set)
		}
	}
	// Session entries apply from the message that adds them onward on its
	// path, the latest per ID, and disabled removes one (Format 11.3).
	disabled := map[string][]bool{} // per ID, each entry adding it on the path
	walkPaths(msgs, func(i int) {
		m := &msgs[i]
		for _, e := range m.Entries {
			disabled[e.ID] = append(disabled[e.ID], e.Disabled)
		}
		for j, ref := range m.Activated {
			owner, id, _ := strings.Cut(ref, "/")
			found := sc.entry(owner, id)
			if owner == "session" {
				d := disabled[id]
				found = len(d) > 0 && !d[len(d)-1]
			}
			if !found {
				c.report(CodeUnknownEntry, inMessage(i, m), ptr("", "activated", j))
			}
		}
	}, func(i int) {
		for _, e := range msgs[i].Entries {
			disabled[e.ID] = disabled[e.ID][:len(disabled[e.ID])-1]
		}
	})
}

// set checks the keys and values of an opening's set or a recorded change, at
// p (Format 8.5). Unknown locations and characters in values are only
// UNKNOWN_LOCATION and UNKNOWN_CHARACTER (Format 15.3).
func (c *checker) set(sc *scope, at place, p jsontext.Pointer, set map[string]jsontext.Value) {
	for key, val := range set {
		kp := ptr(p, key)
		switch key {
		case "scene.location":
			var loc string
			if json.Unmarshal(val, &loc) != nil {
				c.report(CodeInvalidValue, at, kp)
			} else if loc != "" && !c.locations[loc] {
				c.report(CodeUnknownLocation, at, kp)
			}
		case "scene.present":
			var present []string
			if json.Unmarshal(val, &present) != nil {
				c.report(CodeInvalidValue, at, kp)
				continue
			}
			for i, id := range present {
				if c.characters[id] == nil {
					c.report(CodeUnknownCharacter, at, ptr(kp, i))
				}
			}
		default:
			v, ok := sc.variable(key)
			switch {
			case !ok:
				c.report(CodeUnknownVariable, at, kp)
			case !validValue(v, val):
				c.report(CodeInvalidValue, at, kp)
			}
		}
	}
}
