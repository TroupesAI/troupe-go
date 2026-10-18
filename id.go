package troupe

import (
	"encoding/json/jsontext"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// SuggestID returns an ID for an object named name, such as "kai_moreau" for
// "Kai Moreau", that can be added to the list at the JSON Pointer list:
// "/characters", "/worlds", "/scenarios", a world's "/worlds/0/locations", or
// the entries, variables or openings of a character, world or scenario, such
// as "/scenarios/0/openings". The ID follows Format 2.3: it fits the grammar,
// is no reserved word or ID, and is unique in the bundle where the list's
// objects must be.
//
// It is name's letters and digits, lowercased and without accents, with an
// underscore between words, and a number after it when that ID is taken. A
// name that gives none, or that starts with a digit, is given the kind of
// object first, as "character_2" or "scenario_7th_seal". The error is for a
// pointer that names no such list.
func (b *Bundle) SuggestID(list, name string) (string, error) {
	sc, ok := b.idScope(jsontext.Pointer(list))
	if !ok {
		return "", fmt.Errorf("troupe: %q names no list of objects with IDs", list)
	}
	base := slug(name)
	if base == "" || base[0] >= '0' && base[0] <= '9' {
		base = strings.TrimSuffix(sc.kind+"_"+base, "_")
	}
	for n := 1; ; n++ {
		suffix := ""
		if n > 1 {
			suffix = "_" + strconv.Itoa(n)
		}
		id := strings.TrimRight(base[:min(len(base), maxID-len(suffix))], "_") + suffix
		if sc.available(id) {
			return id, nil
		}
	}
}

// maxID is the length of the longest author ID (Format 2.3).
const maxID = 64

// slug returns name's letters and digits, lowercased and without accents,
// with an underscore between words. An apostrophe doesn't end a word, and
// letters and digits an ID can't hold are left out.
func slug(name string) string {
	var b strings.Builder
	gap := false
	for _, r := range norm.NFKD.String(name) {
		switch {
		case 'a' <= r && r <= 'z', '0' <= r && r <= '9':
		case 'A' <= r && r <= 'Z':
			r += 'a' - 'A'
		case r == '\'' || r == '’' || unicode.In(r, unicode.L, unicode.M, unicode.N):
			continue
		default:
			gap = true
			continue
		}
		if gap && b.Len() > 0 {
			b.WriteByte('_')
		}
		gap = false
		b.WriteRune(r)
	}
	return b.String()
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
	// *Location, *Entry, *Variable or *Opening, and ids their IDs.
	objects  []any
	ids      []string
	reserved []string // the IDs they can't have, besides CEL's reserved words
	// taken counts the objects with each ID among those the list's objects
	// must be unique among, theirs included.
	taken map[string]int
}

// idScope returns the scope of the list at p, as SuggestID names lists, and
// reports whether there is one.
func (b *Bundle) idScope(p jsontext.Pointer) (*idScope, bool) {
	if !p.IsValid() {
		return nil, false
	}
	sc := &idScope{taken: map[string]int{}}
	var (
		characterID = func(c *Character) string { return c.ID }
		worldID     = func(w *World) string { return w.ID }
		scenarioID  = func(s *Scenario) string { return s.ID }
		variableID  = func(v *Variable) string { return v.ID }
		locationID  = func(l *Location) string { return l.ID }
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
		entryID := func(e *Entry) string { return e.ID }
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
		openingID := func(o *Opening) string { return o.ID }
		collect(sc, "opening", s.Openings, openingID)
		take(sc, s.Openings, openingID)
	default:
		return nil, false
	}
	return sc, true
}

// collect sets the scope's kind and objects.
func collect[T any](sc *idScope, kind string, objects []T, id func(*T) string) {
	sc.kind = kind
	for i := range objects {
		sc.objects = append(sc.objects, &objects[i])
		sc.ids = append(sc.ids, id(&objects[i]))
	}
}

// take counts the IDs of objects as taken.
func take[T any](sc *idScope, objects []T, id func(*T) string) {
	for i := range objects {
		sc.taken[id(&objects[i])]++
	}
}

// available reports whether an object of the list can take the ID id.
func (sc *idScope) available(id string) bool {
	return authorID.MatchString(id) && !slices.Contains(celReserved, id) && !slices.Contains(sc.reserved, id) && sc.taken[id] == 0
}
