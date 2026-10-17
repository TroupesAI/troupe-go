// Package macro parses text with macros (Format 9).
package macro

import (
	"regexp"
	"slices"
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
			end := strings.Index(text[i+2:], "}}")
			if end >= 0 {
				src := text[i : i+2+end+2]
				if t, ok := form(src, text[i+2:i+2+end]); ok {
					flush()
					if t != nil {
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

// form returns the macro whose source is src and body the text between its
// braces, or nil for a comment, and whether body is a macro at all.
func form(src, body string) (*Token, bool) {
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
		// At least two options, none empty; none can hold :: or }}, so
		// splitting finds them.
		options := strings.Split(body[len("pick::"):], "::")
		if len(options) < 2 || slices.Contains(options, "") {
			return nil, false
		}
		return &Token{Kind: Pick, Text: src, Options: options}, true
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
