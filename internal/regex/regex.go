// Package regex compiles regex keys: patterns in the syntax ECMAScript and RE2
// share, with ECMAScript's meaning under the u flag (Format 14).
package regex

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// The limits of Format 14 on a pattern.
const (
	maxDepth = 100  // groups nested
	maxSize  = 1000 // bytes, written out in full
)

// Regexp is a compiled regex key.
type Regexp struct {
	re   *regexp.Regexp
	fold bool // the i flag
}

// foldWord maps ſ and K (Kelvin) to s and k.
var foldWord = strings.NewReplacer("\u017f", "s", "\u212a", "k")

// MatchString reports whether the key matches anywhere in s, scanned text
// whose every line break is \n (Format 7.2). There RE2 reads ., ^ and $ as
// ECMAScript does.
func (r *Regexp) MatchString(s string) bool {
	if r.fold {
		// Under i, ſ and K are word characters, as s and k are, which RE2's
		// \w and \b don't know. Every set a pattern matches under i holds a
		// character's case variants with it, so mapping them changes no
		// other match.
		s = foldWord.Replace(s)
	}
	return r.re.MatchString(s)
}

// Compile compiles a regex key's pattern and flags. It fails when the key
// isn't valid (Format 14): flags other than i, m and s, or repeated; a form
// outside the grammar; groups nested more than 100 deep; or a pattern longer
// than 1000 bytes with each counted repetition written out.
func Compile(pattern, flags string) (*Regexp, error) {
	p := &parser{src: pattern}
	for _, f := range flags {
		switch {
		case f == 'i' && !p.fold:
			p.fold = true
		case f == 'm' && !p.multiline:
			p.multiline = true
		case f == 's' && !p.dotAll:
			p.dotAll = true
		default:
			return nil, fmt.Errorf("regex: flag %q", f)
		}
	}
	re, size, err := p.alternation()
	if flags != "" {
		// The flags mean the same in RE2, on text whose line breaks are \n.
		flags = "(?" + flags + ")"
	}
	if err == nil && p.pos < len(p.src) {
		err = p.fail("an unescaped )")
	}
	if err != nil {
		return nil, err
	}
	if size > maxSize {
		return nil, errors.New("regex: longer than 1000 bytes written out")
	}
	compiled, err := regexp.Compile(flags + re)
	if err != nil {
		return nil, fmt.Errorf("regex: %w", err)
	}
	return &Regexp{compiled, p.fold}, nil
}

// parser parses a pattern into its RE2 translation and its size written out
// in full, by recursive descent over Format 14's grammar.
type parser struct {
	src                     string
	pos                     int
	depth                   int
	fold, multiline, dotAll bool
}

func (p *parser) fail(what string) error {
	return fmt.Errorf("regex: %s at offset %d", what, p.pos)
}

func (p *parser) peek() rune {
	r, _ := utf8.DecodeRuneInString(p.src[p.pos:])
	return r
}

func (p *parser) next() rune {
	r, n := utf8.DecodeRuneInString(p.src[p.pos:])
	p.pos += n
	return r
}

func (p *parser) eat(s string) bool {
	if strings.HasPrefix(p.src[p.pos:], s) {
		p.pos += len(s)
		return true
	}
	return false
}

// alternation parses sequences separated by |, up to the end or a ).
func (p *parser) alternation() (string, int, error) {
	var b strings.Builder
	size := 0
	for {
		re, n, err := p.sequence()
		if err != nil {
			return "", 0, err
		}
		b.WriteString(re)
		size += n
		if !p.eat("|") {
			return b.String(), min(size, maxSize+1), nil
		}
		b.WriteByte('|')
		size++
	}
}

func (p *parser) sequence() (string, int, error) {
	var b strings.Builder
	size := 0
	for p.pos < len(p.src) && p.peek() != '|' && p.peek() != ')' {
		re, n, quantifiable, err := p.term()
		if err != nil {
			return "", 0, err
		}
		if strings.ContainsRune("*+?{", p.peek()) {
			if !quantifiable {
				return "", 0, p.fail("a quantifier after an assertion")
			}
			if re, n, err = p.quantifier(re, n); err != nil {
				return "", 0, err
			}
		}
		b.WriteString(re)
		size = min(size+n, maxSize+1)
	}
	return b.String(), size, nil
}

// term parses an atom or an assertion, and reports whether a quantifier may
// follow it.
func (p *parser) term() (re string, size int, quantifiable bool, err error) {
	start := p.pos
	switch r := p.next(); r {
	case '^', '$':
		return string(r), 1, false, nil
	case '.':
		return ".", 1, true, nil
	case '(':
		return p.group()
	case '[':
		re, err := p.class()
		return re, p.pos - start, true, err
	case '\\':
		if p.eat("b") || p.eat("B") {
			return p.src[start:p.pos], 2, false, nil
		}
		c, set, err := p.escape(false)
		if set != "" {
			return "[" + set + "]", p.pos - start, true, err
		}
		return quote(c), p.pos - start, true, err
	case ')', ']', '{', '}', '*', '+', '?', '/':
		return "", 0, false, p.fail("an unescaped " + string(r))
	default:
		return quote(r), p.pos - start, true, nil
	}
}

// group parses a group after its (: (…) or (?:…), both translated as RE2's
// non-capturing group, since nothing reads what a group captures.
func (p *parser) group() (string, int, bool, error) {
	open := 1
	if p.eat("?:") {
		open = 3
	} else if p.peek() == '?' {
		return "", 0, false, p.fail("a group other than (…) and (?:…)")
	}
	if p.depth++; p.depth > maxDepth {
		return "", 0, false, p.fail("groups nested more than 100 deep")
	}
	re, n, err := p.alternation()
	if err != nil {
		return "", 0, false, err
	}
	if !p.eat(")") {
		return "", 0, false, p.fail("an unclosed group")
	}
	p.depth--
	return "(?:" + re + ")", open + n + 1, true, nil
}

// quantifier parses the quantifier after an atom translated as re, of size n,
// and returns them together, with their size written out: x{2,3} as xxx, and
// x{2,} as xxx* (Format 14).
func (p *parser) quantifier(re string, n int) (string, int, error) {
	start := p.pos
	size := n + 1
	if p.eat("{") {
		low, ok := p.count()
		if !ok {
			return "", 0, p.fail("an unescaped {")
		}
		high, unbounded := low, false
		if p.eat(",") {
			high, ok = p.count()
			unbounded = !ok
		}
		if !p.eat("}") || !unbounded && (high < low || high == 0) {
			return "", 0, p.fail("a count that isn't valid")
		}
		if unbounded {
			size = low*n + n + 1
		} else {
			size = high * n
		}
	} else {
		p.next()
	}
	if p.eat("?") {
		size++
	}
	return re + p.src[start:p.pos], min(size, maxSize+1), nil
}

// count parses a decimal count, capped past the size limit, so that no
// arithmetic on it overflows.
func (p *parser) count() (int, bool) {
	start := p.pos
	for p.pos < len(p.src) && '0' <= p.src[p.pos] && p.src[p.pos] <= '9' {
		p.pos++
	}
	if p.pos == start {
		return 0, false
	}
	n, err := strconv.Atoi(p.src[start:p.pos])
	if err != nil || n > maxSize+1 {
		n = maxSize + 1
	}
	return n, true
}

// class parses a class after its [ as an RE2 class: characters, class
// escapes, and ranges x-y with x not after y. Only \, [, ], and a - that
// isn't first or last need escaping in it.
func (p *parser) class() (string, error) {
	var b strings.Builder
	b.WriteByte('[')
	if p.eat("^") {
		b.WriteByte('^')
	}
	for first := true; !p.eat("]"); first = false {
		if p.pos >= len(p.src) {
			return "", p.fail("an unclosed class")
		}
		low, set, err := p.classAtom(first)
		if err != nil {
			return "", err
		}
		if set != "" {
			b.WriteString(set)
			continue
		}
		if p.peek() == '-' && !strings.HasPrefix(p.src[p.pos:], "-]") {
			p.next()
			high, highSet, err := p.classAtom(false)
			if err != nil {
				return "", err
			}
			if highSet != "" || high < low {
				return "", p.fail("a range that isn't valid")
			}
			b.WriteString(quote(low) + "-" + quote(high))
			continue
		}
		b.WriteString(quote(low))
	}
	if b.Len() == 1 || b.String() == "[^" {
		return "", p.fail("an empty class")
	}
	return b.String() + "]", nil
}

// classAtom parses a character of a class, or a class escape, returned as the
// body of an RE2 class.
func (p *parser) classAtom(first bool) (rune, string, error) {
	switch r := p.next(); r {
	case '[', ']':
		return 0, "", p.fail("an unescaped " + string(r))
	case '-':
		if !first && p.peek() != ']' {
			return 0, "", p.fail("an unescaped -")
		}
		return r, "", nil
	case '\\':
		return p.escape(true)
	default:
		return r, "", nil
	}
}

// escape parses an escape after its \: a character, or a class escape,
// returned as the body of an RE2 class.
func (p *parser) escape(inClass bool) (rune, string, error) {
	switch r := p.next(); {
	case r == 't':
		return '\t', "", nil
	case r == 'n':
		return '\n', "", nil
	case r == 'r':
		return '\r', "", nil
	case r == 'f':
		return '\f', "", nil
	case r == 'v':
		return '\v', "", nil
	case r == 'd' || r == 'D' || r == 'w' || r == 'W':
		return 0, `\` + string(r), nil
	case r == 's':
		// ECMAScript's white space is Unicode's, and RE2's ASCII.
		return 0, whiteSpace, nil
	case r == 'S':
		return 0, notWhiteSpace, nil
	case r == 'p' || r == 'P':
		if !p.eat("{") {
			return 0, "", p.fail(`a \` + string(r) + " without {")
		}
		end := strings.IndexByte(p.src[p.pos:], '}')
		if end < 0 {
			return 0, "", p.fail(`an unclosed \` + string(r))
		}
		name := p.src[p.pos : p.pos+end]
		p.pos += end + 1
		if !slices.Contains(categories, name) {
			return 0, "", p.fail("a category that isn't valid")
		}
		if r == 'P' && p.fold {
			return 0, foldedComplement(name), nil
		}
		return 0, `\` + string(r) + "{" + name + "}", nil
	case strings.ContainsRune(`^$\.*+?()[]{}|/`, r) || inClass && r == '-':
		return r, "", nil
	default:
		return 0, "", p.fail("an escape that isn't valid")
	}
}

// quote returns r as RE2 matches it literally, in a class or out of one.
func quote(r rune) string {
	if r < ' ' || strings.ContainsRune(`\.+*?()|[]{}^$-`, r) {
		return fmt.Sprintf(`\x{%x}`, r)
	}
	return string(r)
}
