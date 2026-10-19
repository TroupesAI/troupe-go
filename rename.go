package troupe

import (
	"cmp"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/TroupesAI/troupe-go/internal/condition"
	"github.com/TroupesAI/troupe-go/internal/macro"
)

// RenameID returns the JSON Patch (RFC 6902) that gives the object at the
// JSON Pointer object the ID id, and rewrites every reference to it in the
// bundle; for the ID the object has, an empty one. The object is a character,
// world or scenario, such as "/characters/0", a world's location, or an
// entry, variable or opening, such as "/worlds/1/locations/2" or
// "/scenarios/0/variables/3". The patch moves a member it renames to the end
// of its object.
//
// The references are the cast members, scenario worlds, parents, knownBy and
// speakers that name it, the keys of openings' set and the scene values they
// write, and the names macros and conditions read, each resolved as Format 9
// and 10 resolve it in the scenarios that use it. A keyed variable's key that
// is a renamed character's ID, as it is written in of, initial, set, macros and
// conditions, is renamed with it, since it shows as the character's name and
// char looks it up (Format 9, 10.1). In conditions, a string literal is a
// reference when it is compared with char, user, self, scenario, opening,
// scene.location, scene.present, scene.within or players.
//
// It also returns pointers, into the bundle as it is, to the texts,
// conditions and values it leaves as written, whole, since rewriting them
// could change what they mean:
//   - a string value that is a renamed character's or location's ID, which
//     shows as its name (Format 9): an enum's or set's values or labels, an
//     initial value, or a value an opening sets, and a string literal equal
//     to it that a condition uses any other way;
//   - a name in text or a condition of a character or world that refers to
//     the object in one scenario that uses it, and to something else in
//     another;
//   - a string literal that refers to the object but is written with escapes
//     or as a raw string, a condition that doesn't parse and has the ID as a
//     word, and a key whose renamed form its object already has.
//
// The error is for a pointer that names no such object, an ID the object can't
// take (Format 2.3), wrapping ErrIDTaken as CheckID's does, or an object whose
// ID another object shares, so that what references to it refer to is
// ambiguous. In a bundle that fails the schema, a
// value that reads as absent is left as written, and not returned. A session
// played with the bundle isn't changed, so its references to the old ID name
// nothing.
func (b *Bundle) RenameID(object, id string) (jsontext.Value, []string, error) {
	p := jsontext.Pointer(object)
	sc, ok := b.idScope(p.Parent())
	i, isIndex := arrayIndex(p.LastToken())
	if !p.IsValid() || !ok || !isIndex || i >= len(sc.objects) {
		return nil, nil, fmt.Errorf("troupe: %q names no character, world, scenario, location, entry, variable or opening", object)
	}
	old := sc.ids[i]
	if old != "" && sc.taken[old] > 1 {
		return nil, nil, fmt.Errorf("troupe: the ID of %q isn't unique, so what references to it refer to is ambiguous", object)
	}
	if id == old {
		return jsontext.Value("[]"), nil, nil
	}
	if err := sc.check(id); err != nil {
		return nil, nil, err
	}
	r := &renamer{
		names:  newNames(b, nil),
		target: sc.objects[i],
		old:    old,
		id:     id,
		word:   regexp.MustCompile(`\b` + regexp.QuoteMeta(old) + `\b`),
		inUse:  map[any][]*Scenario{},
	}
	// The ID may be absent, in a bundle that fails the schema.
	r.ops = append(r.ops, operation{Op: "add", Path: string(ptr(p, "id")), Value: quote(id)})
	if old != "" {
		r.bundle(b)
	}
	patch, err := json.Marshal(r.ops)
	if err != nil {
		panic(err) // the operations are JSON
	}
	return patch, r.left, nil
}

// operation is an operation of a JSON Patch (RFC 6902).
type operation struct {
	Op    string         `json:"op"`
	From  string         `json:"from,omitempty"`
	Path  string         `json:"path"`
	Value jsontext.Value `json:"value,omitzero"`
}

// renamer finds the references to an object being renamed.
type renamer struct {
	names   *names
	target  any // the object
	old, id string
	word    *regexp.Regexp // the old ID as a word
	// inUse are the scenarios each character and world is in use in.
	inUse map[any][]*Scenario
	ops   []operation
	left  []string
}

// targets reports whether the object being renamed is a T.
func targets[T any](r *renamer) bool {
	_, ok := r.target.(*T)
	return ok
}

func (r *renamer) bundle(b *Bundle) {
	for i := range b.Scenarios {
		sc := newScope(b, nil, i)
		for _, c := range sc.characters {
			r.inUse[c] = append(r.inUse[c], &b.Scenarios[i])
		}
		for _, w := range sc.worlds {
			r.inUse[w] = append(r.inUse[w], &b.Scenarios[i])
		}
	}
	for i := range b.Characters {
		c, p := &b.Characters[i], ptr("", "characters", i)
		in := r.scenarios(c)
		r.text(ptr(p, "description"), c.Description, in, c.ID)
		r.text(ptr(p, "summary"), c.Summary, in, c.ID)
		for j, exchange := range c.Examples {
			for k, l := range exchange {
				r.character(ptr(p, "examples", j, k, "speaker"), l.Speaker)
				r.text(ptr(p, "examples", j, k, "text"), l.Text, in, c.ID)
			}
		}
		r.owner(p, c.Entries, c.Variables, in, c.ID)
	}
	for i := range b.Worlds {
		w, p := &b.Worlds[i], ptr("", "worlds", i)
		in := r.scenarios(w)
		r.text(ptr(p, "description"), w.Description, in, w.ID)
		for j, l := range w.Locations {
			r.location(ptr(p, "locations", j, "parent"), l.Parent)
			r.text(ptr(p, "locations", j, "description"), l.Description, in, w.ID)
		}
		r.owner(p, w.Entries, w.Variables, in, w.ID)
	}
	for i := range b.Scenarios {
		s, p := &b.Scenarios[i], ptr("", "scenarios", i)
		in := []*Scenario{s}
		r.text(ptr(p, "premise"), s.Premise, in, s.ID)
		r.text(ptr(p, "briefing"), s.Briefing, in, s.ID)
		for j, m := range s.Cast {
			r.character(ptr(p, "cast", j, "character"), m.Character)
			r.text(ptr(p, "cast", j, "notes"), m.Notes, in, s.ID)
		}
		if s.Worlds != nil {
			for j, w := range *s.Worlds {
				r.world(ptr(p, "worlds", j), w)
			}
		}
		for j, o := range s.Openings {
			r.set(ptr(p, "openings", j, "set"), s, o.Set)
			for k, part := range o.Content {
				pp := ptr(p, "openings", j, "content", k)
				r.character(ptr(pp, "speaker"), part.Speaker)
				r.characters(ptr(pp, "knownBy"), part.KnownBy)
				r.text(ptr(pp, "text"), part.Text, in, s.ID)
				r.text(ptr(pp, "alt"), part.Alt, in, s.ID)
			}
		}
		r.owner(p, s.Entries, s.Variables, in, s.ID)
	}
}

// scenarios returns the scenarios a character's or world's texts and
// conditions are read in: those it is in use in or, when it is in none, no
// scenario, as a nil one (Format 10.2).
func (r *renamer) scenarios(o any) []*Scenario {
	if in := r.inUse[o]; in != nil {
		return in
	}
	return []*Scenario{nil}
}

// owner rewrites the references in the entries and variables of owner, read
// in the scenarios in.
func (r *renamer) owner(p jsontext.Pointer, entries []Entry, vars []Variable, in []*Scenario, owner string) {
	for i, e := range entries {
		r.characters(ptr(p, "entries", i, "knownBy"), e.KnownBy)
		r.text(ptr(p, "entries", i, "text"), e.Text, in, owner)
		r.condition(ptr(p, "entries", i, "when"), e.When, in, owner)
	}
	for i := range vars {
		r.variable(ptr(p, "variables", i), &vars[i], in, owner)
	}
}

// variable rewrites the references in a variable's description, of and
// initial value, and leaves its values that are the old ID.
func (r *renamer) variable(p jsontext.Pointer, v *Variable, in []*Scenario, owner string) {
	r.text(ptr(p, "description"), v.Description, in, owner)
	for i, s := range v.Values {
		r.value(ptr(p, "values", i), s)
	}
	for _, s := range slices.Sorted(maps.Keys(v.Labels)) {
		r.value(ptr(p, "labels", s), s)
	}
	if targets[Character](r) && slices.Contains(v.Of.List, r.old) {
		i := slices.Index(v.Of.List, r.old)
		if slices.Contains(v.Of.List, r.id) {
			r.leave(ptr(p, "of", i))
		} else {
			r.replace(ptr(p, "of", i), r.id)
		}
	}
	if v.Initial == nil {
		return
	}
	var members map[string]jsontext.Value
	if !v.Of.keyed() || json.Unmarshal(v.Initial, &members) != nil {
		r.values(ptr(p, "initial"), v.Initial)
		return
	}
	for _, k := range slices.Sorted(maps.Keys(members)) {
		r.values(ptr(p, "initial", k), members[k])
		if targets[Character](r) && k == r.old {
			r.move(ptr(p, "initial"), members, k, r.id)
		}
	}
}

// set rewrites the references in an opening's set, in scenario s: its keys
// and the scene values it writes, and leaves its other values that are the
// old ID.
func (r *renamer) set(p jsontext.Pointer, s *Scenario, set map[string]jsontext.Value) {
	for _, key := range slices.Sorted(maps.Keys(set)) {
		switch key {
		case "scene.location":
			var id string
			if json.Unmarshal(set[key], &id) == nil {
				r.location(ptr(p, key), id)
			}
		case "scene.present":
			var ids []string
			if json.Unmarshal(set[key], &ids) == nil {
				r.characters(ptr(p, key), ids)
			}
		default:
			r.values(ptr(p, key), set[key])
			name, ok := strings.CutPrefix(key, "state.")
			if !ok {
				continue
			}
			switch path, safe := r.path([]*Scenario{s}, "", false, strings.Split(name, ".")); {
			case path == nil:
			case !safe:
				r.leave(ptr(p, key))
			default:
				r.move(p, set, key, "state."+strings.Join(path, "."))
			}
		}
	}
}

// text rewrites the state and own paths of the macros in a text of owner,
// read in the scenarios in.
func (r *renamer) text(p jsontext.Pointer, text string, in []*Scenario, owner string) {
	tokens, _ := macro.Parse(text)
	var b strings.Builder
	last := 0
	for _, t := range tokens {
		if t.Kind != macro.State && t.Kind != macro.Own {
			continue
		}
		path, safe := r.path(in, owner, t.Kind == macro.Own, t.Path)
		if path == nil {
			continue
		}
		if !safe {
			r.leave(p)
			return
		}
		prefix := "{{state."
		if t.Kind == macro.Own {
			prefix = "{{own."
		}
		b.WriteString(text[last:t.Offset])
		b.WriteString(prefix)
		b.WriteString(strings.Join(path, "."))
		b.WriteString("}}")
		last = t.Offset + len(t.Text)
	}
	if last > 0 {
		b.WriteString(text[last:])
		r.replace(p, b.String())
	}
}

// condition rewrites the names a condition of owner reads, in the scenarios
// in, and the string literals it compares with IDs.
func (r *renamer) condition(p jsontext.Pointer, expr string, in []*Scenario, owner string) {
	if expr == "" {
		return
	}
	selections, literals, err := condition.Uses(expr)
	if err != nil {
		if r.word.MatchString(expr) {
			r.leave(p)
		}
		return
	}
	type edit struct {
		at  int
		old string
	}
	var edits []edit
	unsafe := false
	for _, s := range selections {
		path, at := s.Names[1:], s.At[1:]
		if s.Key != nil {
			path, at = append(slices.Clone(path), s.Key.Value), append(slices.Clone(at), s.Key.At)
		}
		renamed, safe := r.path(in, owner, s.Names[0] == "own", path)
		for k := range renamed {
			switch {
			case renamed[k] == path[k]:
			case !safe || at[k] < 0:
				unsafe = true
			default:
				edits = append(edits, edit{at[k], path[k]})
			}
		}
	}
	for _, l := range literals {
		switch refers, safe := r.literal(in, l); {
		case !safe || refers && l.At < 0:
			unsafe = true
		case refers:
			edits = append(edits, edit{l.At, l.Value})
		}
	}
	if unsafe {
		r.leave(p)
		return
	}
	if edits == nil {
		return
	}
	slices.SortFunc(edits, func(a, b edit) int { return cmp.Compare(a.at, b.at) })
	var b strings.Builder
	last := 0
	for _, e := range edits {
		b.WriteString(expr[last:e.at])
		b.WriteString(r.id)
		last = e.at + len(e.old)
	}
	b.WriteString(expr[last:])
	r.replace(p, b.String())
}

// literal reports whether a condition's string literal refers to the object,
// by the name it is compared with (Format 10.1), in the scenarios in, and
// whether rewriting it, or leaving it, is safe.
func (r *renamer) literal(in []*Scenario, l condition.Literal) (refers, safe bool) {
	if l.Value != r.old {
		return false, true
	}
	switch l.With {
	case "char", "user", "players", "scene.present":
		return targets[Character](r), true
	case "self":
		return targets[Character](r) || targets[World](r) || targets[Scenario](r), true
	case "scenario":
		return targets[Scenario](r), true
	case "scene.location", "scene.within":
		return targets[Location](r), true
	case "opening":
		// Each scenario that uses the condition has openings of its own.
		safe = true
		for _, s := range in {
			switch o := opening(s, l.Value); {
			case o != nil && o == r.target:
				refers = true
			case o != nil || opening(s, r.id) != nil:
				safe = false
			}
		}
		return refers, safe || !refers
	}
	// It may be compared with a string value that is the ID.
	return false, !targets[Character](r) && !targets[Location](r)
}

// opening returns the opening of s with the ID id, or nil.
func opening(s *Scenario, id string) *Opening {
	if s == nil {
		return nil
	}
	i := slices.IndexFunc(s.Openings, func(o Opening) bool { return o.ID == id })
	if i < 0 {
		return nil
	}
	return &s.Openings[i]
}

// path returns a state or own path, read in a text, condition or key of owner,
// with the names that refer to the object, in any of the scenarios in, as
// the new ID, or nil when none does. It reports whether that is safe: whether,
// in every one of the scenarios, each of those names refers to the object, or
// to nothing as it is and as renamed.
func (r *renamer) path(in []*Scenario, owner string, own bool, path []string) ([]string, bool) {
	var at []int
	for _, s := range in {
		for k, x := range r.refs(s, owner, own, path) {
			if x == r.target && !slices.Contains(at, k) {
				at = append(at, k)
			}
		}
	}
	if at == nil {
		return nil, true
	}
	renamed := slices.Clone(path)
	for _, k := range at {
		renamed[k] = r.id
	}
	for _, s := range in {
		before, after := r.refs(s, owner, own, path), r.refs(s, owner, own, renamed)
		for _, k := range at {
			if before[k] != r.target && (before[k] != nil || after[k] != nil) {
				return renamed, false
			}
		}
	}
	return renamed, true
}

// refs returns what each name of a state or own path refers to, read in a
// text or condition of owner as scenario s plays it (Format 9, 10.2): the
// character or world whose variable it selects, the variable, and the
// character whose ID is the key of a keyed variable; nil for any other name.
func (r *renamer) refs(s *Scenario, owner string, own bool, path []string) []any {
	refs := make([]any, len(path))
	_, v, below, ok := r.names.declared(s, owner, own, path)
	if !ok {
		return refs
	}
	k := len(path) - len(below) - 1
	refs[k] = v
	if k == 1 {
		if c := r.names.characters[path[0]]; c != nil {
			refs[0] = c
		} else {
			refs[0] = r.names.worlds[path[0]]
		}
	}
	if v.Of.keyed() && len(below) > 0 {
		if c := r.names.characters[below[0]]; c != nil {
			refs[k+1] = c
		}
	}
	return refs
}

// character rewrites a reference to a character, at p.
func (r *renamer) character(p jsontext.Pointer, id string) {
	if targets[Character](r) && id == r.old {
		r.replace(p, r.id)
	}
}

// world rewrites a reference to a world, at p.
func (r *renamer) world(p jsontext.Pointer, id string) {
	if targets[World](r) && id == r.old {
		r.replace(p, r.id)
	}
}

// location rewrites a reference to a location, at p.
func (r *renamer) location(p jsontext.Pointer, id string) {
	if targets[Location](r) && id == r.old {
		r.replace(p, r.id)
	}
}

// characters rewrites the references to characters in the list at p.
func (r *renamer) characters(p jsontext.Pointer, ids []string) {
	for i, id := range ids {
		r.character(ptr(p, i), id)
	}
}

// value leaves a string value, at p, that is the ID of a character or
// location being renamed, which shows as its name (Format 9).
func (r *renamer) value(p jsontext.Pointer, s string) {
	if s == r.old && (targets[Character](r) || targets[Location](r)) {
		r.leave(p)
	}
}

// values leaves the string values, at p, of raw, a string or a list, that
// are the ID of a character or location being renamed.
func (r *renamer) values(p jsontext.Pointer, raw jsontext.Value) {
	var x any
	_ = json.Unmarshal(raw, &x) // it read without PARSE
	switch x := x.(type) {
	case string:
		r.value(p, x)
	case []any:
		for i, item := range x {
			if s, ok := item.(string); ok {
				r.value(ptr(p, i), s)
			}
		}
	}
}

// move renames the member from of the object at p, which has members, to
// to, unless it has a member to already.
func (r *renamer) move(p jsontext.Pointer, members map[string]jsontext.Value, from, to string) {
	if _, ok := members[to]; ok {
		r.leave(ptr(p, from))
		return
	}
	r.ops = append(r.ops, operation{Op: "move", From: string(ptr(p, from)), Path: string(ptr(p, to))})
}

func (r *renamer) replace(p jsontext.Pointer, s string) {
	r.ops = append(r.ops, operation{Op: "replace", Path: string(p), Value: quote(s)})
}

// leave records p as a place the renaming leaves as written.
func (r *renamer) leave(p jsontext.Pointer) {
	if !slices.Contains(r.left, string(p)) {
		r.left = append(r.left, string(p))
	}
}

// quote returns s as a JSON string.
func quote(s string) jsontext.Value {
	q, _ := jsontext.AppendQuote(nil, s) // s is valid UTF-8, from the typed model
	return q
}
