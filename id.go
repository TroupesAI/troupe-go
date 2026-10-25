package troupe

import (
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// The errors CheckID's and RenameID's errors wrap, one for each way they
// refuse.
var (
	// ErrBadPointer is for a pointer that names no list or object they take.
	ErrBadPointer = errors.New("bad pointer")
	// ErrIDInvalid is for an ID that doesn't fit the grammar or is reserved
	// (Format 2.3).
	ErrIDInvalid = errors.New("not a valid ID")
	// ErrIDTaken is for an ID another object has where it must be unique
	// (Format 2.3).
	ErrIDTaken = errors.New("the ID is taken")
)

// CheckID reports whether an object added to the list at the JSON Pointer
// list can take the ID id (Format 2.3). The list is "/characters", "/worlds",
// "/scenarios", a world's "/worlds/0/locations", or the entries, variables or
// openings of a character, world or scenario, such as
// "/scenarios/0/openings". The error wraps ErrBadPointer for a pointer that
// names no such list, ErrIDInvalid, or ErrIDTaken when an object the list's
// objects must be unique among has id.
func (b *Bundle) CheckID(list, id string) error {
	sc, ok := b.idScope(jsontext.Pointer(list))
	if !ok {
		return fmt.Errorf("troupe: %w: %q names no list of objects with IDs", ErrBadPointer, list)
	}
	return sc.check(id)
}

// The IDs characters, worlds and scenarios can't have, and those variables
// can't have, besides CEL's reserved words (Format 2.3).
var (
	reservedOwnerIDs    = []string{"char", "user", "narrator", "session", "state", "scene", "players", "own"}
	reservedVariableIDs = []string{"scene", "own"}
)

// idScope is a list of a bundle whose objects have author IDs, and what their
// IDs must avoid (Format 2.3).
type idScope struct {
	kind string // the kind of object, such as "character"
	// objects are the list's objects, as *Character, *World, *Scenario,
	// *Location, *Entry, *Variable or *Opening, and ids their ID fields.
	objects  []any
	ids      []*string
	reserved []string // the IDs they can't have, besides CEL's reserved words
	// taken holds the ID fields of the objects with each ID among those the
	// list's objects must be unique among, theirs included.
	taken map[string][]*string
}

// idScope returns the scope of the list at p, as CheckID names lists, and
// reports whether there is one.
func (b *Bundle) idScope(p jsontext.Pointer) (*idScope, bool) {
	if !p.IsValid() {
		return nil, false
	}
	sc := &idScope{taken: map[string][]*string{}}
	var (
		characterID = func(c *Character) *string { return &c.ID }
		worldID     = func(w *World) *string { return &w.ID }
		scenarioID  = func(s *Scenario) *string { return &s.ID }
		variableID  = func(v *Variable) *string { return &v.ID }
		locationID  = func(l *Location) *string { return &l.ID }
	)
	path := slices.Collect(p.Tokens())
	if len(path) == 1 {
		switch path[0] {
		case "characters":
			collect(sc, "character", b.Characters, characterID)
		case "worlds":
			collect(sc, "world", b.Worlds, worldID)
		case "scenarios":
			collect(sc, "scenario", b.Scenarios, scenarioID)
		default:
			return nil, false
		}
		// Characters, worlds and scenarios are unique among them all, and
		// characters and worlds among scenario variables too.
		sc.reserved = reservedOwnerIDs
		take(sc, b.Characters, characterID)
		take(sc, b.Worlds, worldID)
		take(sc, b.Scenarios, scenarioID)
		if path[0] != "scenarios" {
			for i := range b.Scenarios {
				take(sc, b.Scenarios[i].Variables, variableID)
			}
		}
		return sc, true
	}

	if len(path) != 3 {
		return nil, false
	}
	i, ok := arrayIndex(path[1])
	var (
		entries []Entry
		vars    []Variable
		w       *World
		s       *Scenario
	)
	switch {
	case !ok:
		return nil, false
	case path[0] == "characters" && i < len(b.Characters):
		entries, vars = b.Characters[i].Entries, b.Characters[i].Variables
	case path[0] == "worlds" && i < len(b.Worlds):
		w = &b.Worlds[i]
		entries, vars = w.Entries, w.Variables
	case path[0] == "scenarios" && i < len(b.Scenarios):
		s = &b.Scenarios[i]
		entries, vars = s.Entries, s.Variables
	default:
		return nil, false
	}
	switch {
	case path[2] == "entries":
		entryID := func(e *Entry) *string { return &e.ID }
		collect(sc, "entry", entries, entryID)
		take(sc, entries, entryID)
	case path[2] == "variables":
		collect(sc, "variable", vars, variableID)
		take(sc, vars, variableID)
		sc.reserved = reservedVariableIDs
		if s != nil {
			// A scenario variable's full name would read as an owner's.
			take(sc, b.Characters, characterID)
			take(sc, b.Worlds, worldID)
		}
	case path[2] == "locations" && w != nil:
		collect(sc, "location", w.Locations, locationID)
		// Locations are unique in the bundle.
		for i := range b.Worlds {
			take(sc, b.Worlds[i].Locations, locationID)
		}
	case path[2] == "openings" && s != nil:
		openingID := func(o *Opening) *string { return &o.ID }
		collect(sc, "opening", s.Openings, openingID)
		take(sc, s.Openings, openingID)
	default:
		return nil, false
	}
	return sc, true
}

// collect sets the scope's kind and objects.
func collect[T any](sc *idScope, kind string, objects []T, id func(*T) *string) {
	sc.kind = kind
	for i := range objects {
		sc.objects = append(sc.objects, &objects[i])
		sc.ids = append(sc.ids, id(&objects[i]))
	}
}

// take adds the IDs of objects to those taken.
func take[T any](sc *idScope, objects []T, id func(*T) *string) {
	for i := range objects {
		p := id(&objects[i])
		sc.taken[*p] = append(sc.taken[*p], p)
	}
}

// check reports whether an object of the list can take the ID id.
func (sc *idScope) check(id string) error {
	switch {
	case !authorID.MatchString(id) || slices.Contains(celReserved, id) || slices.Contains(sc.reserved, id):
		return fmt.Errorf("troupe: %q: %w for a %s", id, ErrIDInvalid, sc.kind)
	case len(sc.taken[id]) > 0:
		return fmt.Errorf("troupe: %q: %w", id, ErrIDTaken)
	}
	return nil
}

// arrayIndex reads tok as an array index: 0, or digits that don't start with
// 0 (RFC 6901 section 4).
func arrayIndex(tok string) (int, bool) {
	if tok == "" || tok[0] == '0' && tok != "0" || strings.Trim(tok, "0123456789") != "" {
		return 0, false
	}
	i, err := strconv.Atoi(tok)
	return i, err == nil
}
