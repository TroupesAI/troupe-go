package troupe

import (
	"fmt"
	"maps"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"

	"github.com/TroupesAI/troupe-go/internal/macro"
)

// RenderContext is where author text is rendered (Format 9).
type RenderContext struct {
	// State is the state the macros read, a state of the story: at the
	// message the text is in, or elsewhere at the evaluation point, the
	// session's head, or the initial state when there is no head (Format 9).
	// {{user}} comes from it too.
	State State
	// Char is the character {{char}} means: a part's speaker, the character
	// the text belongs to, or the one the message being written is for. ""
	// means none.
	Char string
	// Narrator reports whether the text is for a message written for the
	// narrator, so {{char}}, without Char, is the scenario's narrator.
	Narrator bool
	// Owner is the text's owner, which own means: a character, world or
	// scenario ID, or "session" or "" for a session entry, which has none
	// (Format 8.2).
	Owner string
}

// Render returns author text with its macros resolved (Format 9): the texts
// of a bundle, and of a session's characters, worlds and entries, where
// macros resolve. A message part's text is RenderParts's to show, since a
// player's or model's words are used as written. r chooses each pick's
// option, so rendering is reproducible from the caller's source.
//
// Text between {{ and }} that is no macro, or that names an undeclared
// variable, is left as written; a declared variable with nothing to show
// renders as nothing.
func (st *Story) Render(text string, c RenderContext, r *rand.Rand) string {
	sc := st.scope
	own := sc.all.ownOf(sc.scenario, c.Owner)
	tokens, _ := macro.Parse(text)
	var b strings.Builder
	for _, t := range tokens {
		switch t.Kind {
		case macro.Text:
			b.WriteString(t.Text)
		case macro.Char:
			switch {
			case c.Char != "":
				b.WriteString(sc.characterName(c.Char))
			case c.Narrator:
				b.WriteString(sc.narrator())
			default:
				b.WriteString(t.Text)
			}
		case macro.User:
			if u := c.State.s.user; u != "" {
				b.WriteString(sc.characterName(u))
			} else {
				b.WriteString(t.Text)
			}
		case macro.Location:
			if l := sc.all.locations[c.State.s.location]; l != nil {
				b.WriteString(l.Name)
			} else {
				b.WriteString(c.State.s.location)
			}
		case macro.Present:
			b.WriteString(sc.characterNames(c.State.s.present))
		case macro.Pick:
			b.WriteString(t.Options[r.IntN(len(t.Options))])
		case macro.State, macro.Own:
			name, v, below, ok := sc.all.declared(sc.scenario, own, t.Kind == macro.Own, t.Path)
			if !ok {
				b.WriteString(t.Text)
				continue
			}
			b.WriteString(sc.renderAt(c, v, c.State.s.values[name], below))
		}
	}
	return b.String()
}

// RenderParts returns m's parts as they are shown (Format 9), given this
// story's state before m, as KnownBy and Apply take it. In a root made from
// an opening, their text and alt resolve at the root, with each part's
// speaker as {{char}} and own meaning state; r chooses a pick's option,
// though a root has its picks chosen already (Format 11.1). Any other message
// is a player's or a model's words, used as written, so no state leaks
// through model output or prompt attacks (Format Appendix C).
func (st *Story) RenderParts(before State, m *Message, r *rand.Rand) []Part {
	return st.renderParts(st.Apply(before, m), m, r)
}

// renderParts returns m's parts as they are shown, given the state at m.
func (st *Story) renderParts(at State, m *Message, r *rand.Rand) []Part {
	parts := slices.Clone(m.Content)
	if m.Parent != "" || m.Opening == "" {
		return parts
	}
	for i := range parts {
		c := RenderContext{State: at, Char: parts[i].Speaker, Owner: st.scope.scenario.ID}
		parts[i].Text = st.Render(parts[i].Text, c, r)
		parts[i].Alt = st.Render(parts[i].Alt, c, r)
	}
	return parts
}

// Escape returns text written so that, where macros resolve, it resolves to
// exactly text: each \ as \\ and each {{ as \{{ (Format 9). A writer escapes
// a player's or a model's words before putting them into a session's
// characters, worlds or entries, in the fields where macros resolve, such as
// a description or an entry's text, but not in those used as written, such
// as a name.
func Escape(text string) string {
	return escaper.Replace(text)
}

// escaper escapes in one left-to-right pass, so {{{ is \{{{.
var escaper = strings.NewReplacer(`\`, `\\`, "{{", `\{{`)

// renderAt renders the part of a variable's value x that the path's segments
// below the variable name: x itself, or a keyed variable's key, where char
// and user mean the macros' characters (Format 9). It renders as nothing when
// there is nothing there: an absent key, a path below a value that isn't a
// map, or a variable whose owner isn't in use, which the state doesn't hold.
func (sc *scope) renderAt(c RenderContext, v *Variable, x any, below []string) string {
	if x == nil || len(below) > 1 {
		return ""
	}
	m, keyed := x.(map[string]any)
	if len(below) == 0 {
		if keyed {
			return sc.renderMap(v, m)
		}
		return sc.render(v, x)
	}
	if !keyed {
		return ""
	}
	key := below[0]
	switch key {
	case "char":
		key = c.Char
	case "user":
		key = c.State.s.user
	}
	if y, ok := m[key]; ok && key != "" {
		return sc.render(v, y)
	}
	return ""
}

// render renders a value of v (Format 9): a label, a character's name, a
// location's name, or as written; numbers in decimal; lists joined by ", ".
func (sc *scope) render(v *Variable, x any) string {
	switch x := x.(type) {
	case bool:
		return sc.word(v, strconv.FormatBool(x))
	case int64:
		return strconv.FormatInt(x, 10)
	case string:
		return sc.word(v, x)
	case []string:
		words := make([]string, len(x))
		for i, s := range x {
			words[i] = sc.word(v, s)
		}
		return strings.Join(words, ", ")
	}
	// Only a variable of a type the spec doesn't define holds anything else.
	return fmt.Sprint(x)
}

// renderMap renders a keyed variable's value as "Name: value" pairs, in the
// order of its keys: the cast's, the list's, or code point order for a "*"
// map (Format 8.3). A key that is a character ID shows the character's name,
// and any other its underscores as spaces.
func (sc *scope) renderMap(v *Variable, m map[string]any) string {
	keys := sc.keys(v)
	if v.Of.Any {
		keys = slices.Sorted(maps.Keys(m))
	}
	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		name := strings.ReplaceAll(k, "_", " ")
		if c := sc.all.characters[k]; c != nil {
			name = c.Name
		}
		pairs = append(pairs, name+": "+sc.render(v, m[k]))
	}
	return strings.Join(pairs, ", ")
}

// word renders a string value: its label, else the name of the character it
// is the ID of, else that of the location, else the string.
func (sc *scope) word(v *Variable, s string) string {
	if label, ok := v.Labels[s]; ok {
		return label
	}
	if c := sc.all.characters[s]; c != nil {
		return c.Name
	}
	if l := sc.all.locations[s]; l != nil {
		return l.Name
	}
	return s
}

func (sc *scope) characterName(id string) string {
	if c := sc.all.characters[id]; c != nil {
		return c.Name
	}
	return id
}

func (sc *scope) characterNames(ids []string) string {
	names := make([]string, len(ids))
	for i, id := range ids {
		names[i] = sc.characterName(id)
	}
	return strings.Join(names, ", ")
}

// narrator returns what text calls the narrator (Format 6).
func (sc *scope) narrator() string {
	if sc.scenario.Narrator != "" {
		return sc.scenario.Narrator
	}
	return "the narrator"
}
