package troupe

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"slices"
	"strconv"
	"strings"

	"github.com/TroupesAI/troupe-go/internal/condition"
	"github.com/TroupesAI/troupe-go/internal/keys"
	"github.com/TroupesAI/troupe-go/internal/macro"
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
		b:             b,
		names:         newNames(b, nil),
		seenOwners:    map[string]bool{},
		seenLocations: map[string]bool{},
		envs:          map[any]*condition.Env{},
	}
	c.owners(inBundle, b.Characters, b.Worlds)
	c.scenarios()
	if s != nil {
		c.s = s
		c.add(s.Header.Characters, s.Header.Worlds)
		c.owners(inHeader, s.Header.Characters, s.Header.Worlds)
		c.session()
	}
	return sortDiagnostics(c.diags)
}

type checker struct {
	b *Bundle
	s *Session // nil until the session's checks
	// names are what references can name: the bundle's, and once its checks
	// begin, the session's.
	*names
	// seenOwners and seenLocations are the IDs seen so far, for DUPLICATE_ID:
	// those of characters, worlds and scenarios, and those of locations.
	// Duplicates are reported at each ID after the first, in the order of
	// conformance README section 3.4.
	seenOwners    map[string]bool
	seenLocations map[string]bool
	// envs are the environments of conditions, by scenario and owner, and
	// whether the session's names are in them.
	envs  map[any]*condition.Env
	diags []Diagnostic
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
	return place{line: i + 2, message: validID(m.ID)}
}

// validID returns id when it is a valid message ID, and "" otherwise, so a
// diagnostic's Message is always an ID.
func validID(id string) string {
	if messageID.MatchString(id) {
		return id
	}
	return ""
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

// owners checks the characters and worlds of the document at, the bundle or a
// session's header: their IDs and those of their locations, entries and
// variables (Format 2.3), their references, and their variables' initial
// values.
func (c *checker) owners(at place, characters []Character, worlds []World) {
	for i := range characters {
		ch, p := &characters[i], ptr("", "characters", i)
		c.unique(c.seenOwners, ch.ID, at, ptr(p, "id"))
		c.ownIDs(at, p, ch.Entries, ch.Variables)
		for j, exchange := range ch.Examples {
			for k, l := range exchange {
				if l.Speaker != "" && l.Speaker != "char" && l.Speaker != "user" && c.characters[l.Speaker] == nil {
					c.report(CodeUnknownCharacter, at, ptr(p, "examples", j, k, "speaker"))
				}
			}
		}
		c.entries(at, p, ch.Entries)
		c.initials(at, p, ch.Variables)
	}
	for i := range worlds {
		w, p := &worlds[i], ptr("", "worlds", i)
		c.unique(c.seenOwners, w.ID, at, ptr(p, "id"))
		for j, l := range w.Locations {
			c.unique(c.seenLocations, l.ID, at, ptr(p, "locations", j, "id"))
		}
		c.ownIDs(at, p, w.Entries, w.Variables)
		c.entries(at, p, w.Entries)
		c.initials(at, p, w.Variables)
	}
	c.parents(at)
}

// scenarios checks the bundle's scenarios: their IDs and those of their
// entries, variables and openings, what they name, their variables' initial
// values, and the keys and values of their openings as each is written. It
// checks the macros of each scenario and of the characters and worlds it has
// in use against its variables, and those of characters and worlds no
// scenario uses against none (Format 10.2).
func (c *checker) scenarios() {
	b := c.b
	usedCharacters, usedWorlds := make([]bool, len(b.Characters)), make([]bool, len(b.Worlds))
	for i := range c.b.Scenarios {
		sc, p := &c.b.Scenarios[i], ptr("", "scenarios", i)
		c.unique(c.seenOwners, sc.ID, inBundle, ptr(p, "id"))
		c.ownIDs(inBundle, p, sc.Entries, sc.Variables)
		for j, v := range sc.Variables {
			// A scenario variable's full name would read as an owner's.
			if c.characters[v.ID] != nil || c.worlds[v.ID] != nil {
				c.report(CodeDuplicateID, inBundle, ptr(p, "variables", j, "id"))
			}
		}
		// A scenario names each character and world once.
		cast, worlds, openings := map[string]bool{}, map[string]bool{}, map[string]bool{}
		for j, m := range sc.Cast {
			c.unique(cast, m.Character, inBundle, ptr(p, "cast", j, "character"))
			if m.Character != "" && c.characters[m.Character] == nil {
				c.report(CodeUnknownCharacter, inBundle, ptr(p, "cast", j, "character"))
			}
		}
		if sc.Worlds != nil {
			for j, w := range *sc.Worlds {
				c.unique(worlds, w, inBundle, ptr(p, "worlds", j))
				if c.worlds[w] == nil {
					c.report(CodeUnknownWorld, inBundle, ptr(p, "worlds", j))
				}
			}
		}
		for j, o := range sc.Openings {
			c.unique(openings, o.ID, inBundle, ptr(p, "openings", j, "id"))
			c.parts(inBundle, ptr(p, "openings", j, "content"), o.Content)
		}
		c.entries(inBundle, p, sc.Entries)
		c.initials(inBundle, p, sc.Variables)
		scope := newScope(b, nil, i)
		c.openings(scope)

		c.text(inBundle, ptr(p, "premise"), sc.Premise, sc, sc.ID)
		c.text(inBundle, ptr(p, "briefing"), sc.Briefing, sc, sc.ID)
		for j, m := range sc.Cast {
			c.text(inBundle, ptr(p, "cast", j, "notes"), m.Notes, sc, sc.ID)
		}
		c.ownerUses(inBundle, p, sc.Entries, sc.Variables, sc, sc.ID)
		for j, o := range sc.Openings {
			c.partUses(inBundle, ptr(p, "openings", j, "content"), o.Content, sc, sc.ID)
		}
		for j := range b.Characters {
			if scope.characters[b.Characters[j].ID] == &b.Characters[j] {
				usedCharacters[j] = true
				c.characterUses(inBundle, ptr("", "characters", j), &b.Characters[j], sc)
			}
		}
		for j := range b.Worlds {
			if scope.worlds[b.Worlds[j].ID] == &b.Worlds[j] {
				usedWorlds[j] = true
				c.worldUses(inBundle, ptr("", "worlds", j), &b.Worlds[j], sc)
			}
		}
	}
	for j := range b.Characters {
		if !usedCharacters[j] {
			c.characterUses(inBundle, ptr("", "characters", j), &b.Characters[j], nil)
		}
	}
	for j := range b.Worlds {
		if !usedWorlds[j] {
			c.worldUses(inBundle, ptr("", "worlds", j), &b.Worlds[j], nil)
		}
	}
}

// session checks a session's header and messages, after its characters and
// worlds, against its scenario.
func (c *checker) session() {
	b, s, h := c.b, c.s, &c.s.Header
	// A session's character or world that takes a scenario variable's ID is
	// the session's to report, since the bundle is valid without it.
	for i, ch := range h.Characters {
		c.scenarioVariable(ch.ID, ptr("", "characters", i, "id"))
	}
	for i, w := range h.Worlds {
		c.scenarioVariable(w.ID, ptr("", "worlds", i, "id"))
	}
	scenario := scenarioIndex(b, h.Scenario)
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
		// A parent is on an earlier line (Format 11.1).
		if m.Parent != "" && !ids[m.Parent] {
			c.report(CodeUnknownMessage, at, "/parent")
		}
		c.unique(ids, m.ID, at, "/id")
		if m.Char != "" && c.characters[m.Char] == nil {
			c.report(CodeUnknownCharacter, at, "/char")
		}
		c.parts(at, "/content", m.Content)
		c.ownIDs(at, "", m.Entries, nil)
		c.entries(at, "", m.Entries)
	}
	if h.Head != "" && !ids[h.Head] {
		c.report(CodeUnknownMessage, inHeader, "/head")
	}
	scope := newScope(b, s, scenario)
	c.messages(scope)

	// A session's characters, worlds and entries are checked against its
	// scenario, and so are roots made from its openings, where own means
	// state. A session entry has no own (Format 8.2).
	for i := range h.Characters {
		c.characterUses(inHeader, ptr("", "characters", i), &h.Characters[i], scope.scenario)
	}
	for i := range h.Worlds {
		c.worldUses(inHeader, ptr("", "worlds", i), &h.Worlds[i], scope.scenario)
	}
	for i := range s.Messages {
		m := &s.Messages[i]
		c.ownerUses(inMessage(i, m), "", m.Entries, nil, scope.scenario, "")
		if m.Parent == "" && m.Opening != "" {
			c.partUses(inMessage(i, m), "/content", m.Content, scope.scenario, h.Scenario)
		}
	}
}

// characterUses checks the macros in a character's texts, and its entries'
// conditions, owned by it, against scenario's variables.
func (c *checker) characterUses(at place, p jsontext.Pointer, ch *Character, scenario *Scenario) {
	c.text(at, ptr(p, "description"), ch.Description, scenario, ch.ID)
	c.text(at, ptr(p, "summary"), ch.Summary, scenario, ch.ID)
	for i, exchange := range ch.Examples {
		for j, l := range exchange {
			c.text(at, ptr(p, "examples", i, j, "text"), l.Text, scenario, ch.ID)
		}
	}
	c.ownerUses(at, p, ch.Entries, ch.Variables, scenario, ch.ID)
}

// worldUses checks the macros in a world's texts, and its entries'
// conditions, owned by it, against scenario's variables.
func (c *checker) worldUses(at place, p jsontext.Pointer, w *World, scenario *Scenario) {
	c.text(at, ptr(p, "description"), w.Description, scenario, w.ID)
	for i, l := range w.Locations {
		c.text(at, ptr(p, "locations", i, "description"), l.Description, scenario, w.ID)
	}
	c.ownerUses(at, p, w.Entries, w.Variables, scenario, w.ID)
}

// ownerUses checks the macros in the texts of an owner's entries and the
// descriptions of its variables, and its entries' conditions.
func (c *checker) ownerUses(at place, p jsontext.Pointer, entries []Entry, vars []Variable, scenario *Scenario, owner string) {
	for i, e := range entries {
		c.text(at, ptr(p, "entries", i, "text"), e.Text, scenario, owner)
		c.condition(at, ptr(p, "entries", i, "when"), e.When, scenario, owner)
	}
	for i, v := range vars {
		c.text(at, ptr(p, "variables", i, "description"), v.Description, scenario, owner)
	}
}

// partUses checks the macros in the text and alt of the parts at p.
func (c *checker) partUses(at place, p jsontext.Pointer, parts []Part, scenario *Scenario, owner string) {
	for i, part := range parts {
		c.text(at, ptr(p, i, "text"), part.Text, scenario, owner)
		c.text(at, ptr(p, i, "alt"), part.Alt, scenario, owner)
	}
}

// condition checks a condition, owned by owner, against scenario's variables,
// when it isn't nil, and those of every character and world (Format 10.2): a
// condition that doesn't compile is CEL_ERROR.
func (c *checker) condition(at place, p jsontext.Pointer, expr string, scenario *Scenario, owner string) {
	if expr == "" {
		return
	}
	env, err := c.env(scenario, owner)
	if err == nil {
		_, err = env.Compile(expr)
	}
	if err != nil {
		c.report(CodeCELError, at, p)
	}
}

// env returns the environment of an owner's conditions, against scenario's
// variables, made once per scenario and owner while the names stay the same.
func (c *checker) env(scenario *Scenario, owner string) (*condition.Env, error) {
	type key struct {
		scenario    *Scenario
		owner       string
		withSession bool
	}
	k := key{scenario, owner, c.s != nil}
	if e, ok := c.envs[k]; ok {
		return e, nil
	}
	base, ok := c.envs[key{scenario, "", c.s != nil}]
	if !ok {
		var err error
		if base, err = condition.NewEnv(declarations(scenario, c.names)); err != nil {
			return nil, err
		}
		c.envs[key{scenario, "", c.s != nil}] = base
	}
	e, err := base.With(ownDeclarations(scenario, c.names, owner))
	if err != nil {
		return nil, err
	}
	c.envs[k] = e
	return e, nil
}

// text checks the macros in a text where they resolve, at p: each is a macro,
// and names a declared variable, as Format 10.2 counts declarations with the
// variables of scenario, when it isn't nil (Format 9, 15.3).
func (c *checker) text(at place, p jsontext.Pointer, text string, scenario *Scenario, owner string) {
	tokens, unknown := macro.Parse(text)
	for _, t := range tokens {
		if t.Kind == macro.State || t.Kind == macro.Own {
			if _, _, _, ok := c.declared(scenario, owner, t.Kind == macro.Own, t.Path); !ok {
				unknown = true
			}
		}
	}
	if unknown {
		c.report(CodeUnknownMacro, at, p)
	}
}

// scenarioVariable reports a session's character or world, at p, that takes a
// scenario variable's ID (Format 2.3).
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

// entries checks the knownBy and keys of the entries of the owner at p: a
// regex key that isn't valid is REGEX_ERROR (Format 14).
func (c *checker) entries(at place, p jsontext.Pointer, entries []Entry) {
	for i, e := range entries {
		c.knownBy(at, ptr(p, "entries", i, "knownBy"), e.KnownBy)
		for j, k := range e.Keys {
			if _, err := keys.Compile(k); err != nil {
				c.report(CodeRegexError, at, ptr(p, "entries", i, "keys", j))
			}
		}
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

// initials checks the initial values of the variables of the owner at p
// (Format 8.1, 8.3, 8.5).
func (c *checker) initials(at place, p jsontext.Pointer, vars []Variable) {
	for i := range vars {
		c.initial(at, ptr(p, "variables", i), &vars[i])
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
	values := index(v.Values)
	if !v.Of.keyed() || v.Initial.Kind() != '{' {
		if !validValue(v, values, v.Initial) {
			c.report(CodeInvalidValue, at, p)
		}
		return
	}
	var members map[string]jsontext.Value
	if err := json.Unmarshal(v.Initial, &members); err != nil {
		panic(err) // an object that read without PARSE
	}
	keys := index(v.Of.List)
	for k, val := range members {
		_, listed := keys[k]
		switch {
		case k == "*":
		case v.Of.Cast && c.characters[k] == nil:
			c.report(CodeUnknownCharacter, at, ptr(p, k))
			continue
		case keys != nil && !listed:
			continue
		}
		if !validValue(v, values, val) {
			c.report(CodeInvalidValue, at, ptr(p, k))
		}
	}
}

// openings checks the keys and values of the openings of sc's scenario.
func (c *checker) openings(sc *scope) {
	initial := sc.initial()
	for j, o := range sc.scenario.Openings {
		p := ptr("", "scenarios", sc.index, "openings", j, "set")
		c.set(sc, inBundle, p, o.Set, sc.openingVariable)
		c.mapLimit(sc, inBundle, p, o.Set, sc.apply(initial.clone(), o.Set, nil))
	}
}

// mapLimit reports the keys of set that write to a "*" map in over, which a
// change left with more than maxSetMembers keys (Format 8.5, 14).
func (c *checker) mapLimit(sc *scope, at place, p jsontext.Pointer, set map[string]jsontext.Value, over []string) {
	for key := range set {
		if b, _, ok := sc.variable(key); ok && slices.Contains(over, b.name) {
			c.report(CodeInvalidValue, at, ptr(p, key))
		}
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
			c.set(sc, inMessage(i, m), ptr("", "changes", j, "set"), ch.Set, sc.variable)
		}
	}
	// The state along each path, for the "*" map limit. Each message's
	// changes are undone on the way back up.
	st := sc.initial()
	undo := make([][]edit, len(msgs))
	// Session entries apply from the message that adds them onward on its
	// path, the latest per ID, and disabled removes one (Format 11.3).
	disabled := map[string][]bool{} // per ID, each entry adding it on the path
	walkPaths(msgs, func(i int) {
		m := &msgs[i]
		for j, ch := range m.Changes {
			over := sc.apply(st, ch.Set, &undo[i])
			c.mapLimit(sc, inMessage(i, m), ptr("", "changes", j, "set"), ch.Set, over)
		}
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
		st.revert(undo[i])
		undo[i] = nil
		for _, e := range msgs[i].Entries {
			disabled[e.ID] = disabled[e.ID][:len(disabled[e.ID])-1]
		}
	})
}

// set checks the keys and values of an opening's set or a recorded change, at
// p, with variable resolving its keys (Format 8.5). Unknown locations and
// characters in values are only UNKNOWN_LOCATION and UNKNOWN_CHARACTER
// (Format 15.3).
func (c *checker) set(sc *scope, at place, p jsontext.Pointer, set map[string]jsontext.Value, variable func(string) (*binding, string, bool)) {
	for key, val := range set {
		kp := ptr(p, key)
		switch key {
		case "scene.location":
			var loc string
			if json.Unmarshal(val, &loc) != nil {
				c.report(CodeInvalidValue, at, kp)
			} else if loc != "" && c.locations[loc] == nil {
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
			b, _, ok := variable(key)
			switch {
			case !ok:
				c.report(CodeUnknownVariable, at, kp)
			case !validValue(b.v, b.values, val):
				c.report(CodeInvalidValue, at, kp)
			}
		}
	}
}
