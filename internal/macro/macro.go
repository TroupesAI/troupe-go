// Package macro parses text with macros (Format 9).
package macro

import (
	"regexp"
	"strings"
)

// Kind is the kind of a Token.
type Kind int

// The kinds of token. A comment is no token: it renders as nothing.
const (
	Text     Kind = iota // literal text, escapes resolved
	Char                 // {{char}}
	User                 // {{user}}
	State                // {{state.<path>}}
	Own                  // {{own.<path>}}
	Location             // {{scene.location}}
	Present              // {{scene.present}}
	Pick                 // {{pick::<a>::<b>…}}
)

// Token is literal text or a macro.
type Token struct {
	Kind    Kind
	Text    string   // the text of a Text token; a macro's source, braces included
	Offset  int      // where a macro's source starts in the text
	Path    []string // the segments of a State or Own path
	Options []string // a Pick's options
}

// path is a state path: author IDs joined by dots (Format 2.3).
var path = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*$`)

// Parse parses text in one left-to-right scan (Format 9). At each {{, the
// form up to the next }} is a macro when it is one of the table's, with no
// spaces outside comments and options; otherwise the first { is kept and the
// scan resumes at the next character. \{{ is a literal {{, and \\ a literal \.
//
// unknown reports whether text has a {{ that is no macro, where the text
// from it to the next }} holds no macro the scan finds: {{a{{char}} and
// {{{char}} hold one, and {{foo}} doesn't.
func Parse(text string) (tokens []Token, unknown bool) {
	var lit strings.Builder
	flush := func() {
		if lit.Len() > 0 {
			tokens = append(tokens, Token{Kind: Text, Text: lit.String()})
			lit.Reset()
		}
	}
	// open is where the text from the first {{ that is no macro to the next
	// }} ends, while the scan has found no macro in it; -1 otherwise.
	open := -1
	// r is the text up to the next }}, which every {{ before it shares, so it
	// is searched once, however many {{ the scan tries.
	var r region
	for i := 0; i < len(text); {
		if open >= 0 && i >= open {
			unknown, open = true, -1
		}
		switch {
		case strings.HasPrefix(text[i:], `\\`):
			lit.WriteByte('\\')
			i += 2
		case strings.HasPrefix(text[i:], `\{{`):
			lit.WriteString("{{")
			i += 3
		case strings.HasPrefix(text[i:], "{{"):
			if r.end < i+2 {
				r = find(text, i+2)
			}
			if r.end < len(text) {
				src, body := text[i:r.end+2], text[i+2:r.end]
				options := strings.HasPrefix(body, "pick::") && r.options(text, i+2+len("pick::"))
				if t, ok := form(src, body, options); ok {
					flush()
					if t != nil {
						t.Offset = i
						tokens = append(tokens, *t)
					}
					i += len(src)
					open = -1
					continue
				}
				if open < 0 {
					open = i + len(src)
				}
			}
			lit.WriteByte('{')
			i++
		default:
			lit.WriteByte(text[i])
			i++
		}
	}
	flush()
	return tokens, unknown || open >= 0
}

// region is the text from a {{ to the next }}.
type region struct {
	end   int // where the }} starts; len(text) when there is none
	sep   int // where the last :: in the region starts; -1 for none
	quad  int // where the last :::: in the region starts; -1 for none
	colon int // the colons right before end, in the region
}

// find returns the region from start to the next }}.
func find(text string, start int) region {
	end := strings.Index(text[start:], "}}")
	if end < 0 {
		return region{end: len(text)}
	}
	r := region{end: start + end, sep: -1, quad: -1}
	in := text[start:r.end]
	if i := strings.LastIndex(in, "::"); i >= 0 {
		r.sep = start + i
	}
	if i := strings.LastIndex(in, "::::"); i >= 0 {
		r.quad = start + i
	}
	for r.colon < len(in) && in[len(in)-1-r.colon] == ':' {
		r.colon++
	}
	return r
}

// options reports whether text[s:r.end] is a pick's options: at least two,
// none empty, as splitting it at each :: from the left gives them. Splitting
// takes the colons of each run two at a time from its start, so an option is
// empty only at a run of two or more at the start, an even run at the end,
// or a run of four or more, which the region's ends answer at once.
func (r *region) options(text string, s int) bool {
	if r.sep < s || r.quad >= s || strings.HasPrefix(text[s:], "::") {
		return false
	}
	colons := min(r.colon, r.end-s)
	return colons < 2 || colons%2 == 1
}

// form returns the macro whose source is src and body the text between its
// braces, or nil for a comment, and whether body is a macro at all. options
// reports whether a pick's body holds valid options.
func form(src, body string, options bool) (*Token, bool) {
	switch {
	case strings.HasPrefix(body, "//"):
		return nil, true
	case body == "char":
		return &Token{Kind: Char, Text: src}, true
	case body == "user":
		return &Token{Kind: User, Text: src}, true
	case body == "scene.location":
		return &Token{Kind: Location, Text: src}, true
	case body == "scene.present":
		return &Token{Kind: Present, Text: src}, true
	case strings.HasPrefix(body, "pick::"):
		if !options {
			return nil, false
		}
		return &Token{Kind: Pick, Text: src, Options: strings.Split(body[len("pick::"):], "::")}, true
	}
	for _, k := range []struct {
		prefix string
		kind   Kind
	}{{"state.", State}, {"own.", Own}} {
		if p, ok := strings.CutPrefix(body, k.prefix); ok && path.MatchString(p) {
			return &Token{Kind: k.kind, Text: src, Path: strings.Split(p, ".")}, true
		}
	}
	return nil, false
}
