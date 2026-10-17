package regex

import (
	"fmt"
	"strings"
	"sync"
	"unicode"
)

// lineTerminators are ECMAScript's, as the body of an RE2 class.
const lineTerminators = `\n\r\x{2028}\x{2029}`

// spaces are ECMAScript's white space and line terminators, which \s matches:
// tab, vertical tab, form feed, U+FEFF and Unicode's space separators, with
// the line terminators.
var spaces = []rune{
	'\t', '\n', '\v', '\f', '\r', ' ', 0xa0, 0x1680,
	0x2000, 0x2001, 0x2002, 0x2003, 0x2004, 0x2005, 0x2006, 0x2007, 0x2008, 0x2009, 0x200a,
	0x2028, 0x2029, 0x202f, 0x205f, 0x3000, 0xfeff,
}

// whiteSpace and notWhiteSpace are the bodies of RE2 classes for \s and \S.
var whiteSpace, notWhiteSpace = spaceBodies()

func spaceBodies() (string, string) {
	var in, out strings.Builder
	next := rune(0) // the first code point not yet in out
	for _, r := range spaces {
		fmt.Fprintf(&in, `\x{%x}`, r)
		if r > next {
			fmt.Fprintf(&out, `\x{%x}-\x{%x}`, next, r-1)
		}
		next = r + 1
	}
	fmt.Fprintf(&out, `\x{%x}-\x{%x}`, next, unicode.MaxRune)
	return in.String(), out.String()
}

// categories are the general categories \p and \P may name: all but Cn and LC
// (Format 14).
var categories = []string{
	"L", "Lu", "Ll", "Lt", "Lm", "Lo",
	"M", "Mn", "Mc", "Me",
	"N", "Nd", "Nl", "No",
	"P", "Pc", "Pd", "Ps", "Pe", "Pi", "Pf", "Po",
	"S", "Sm", "Sc", "Sk", "So",
	"Z", "Zs", "Zl", "Zp",
	"C", "Cc", "Cf", "Cs", "Co",
}

var foldedComplements sync.Map // category name → class body

// foldedComplement returns the body of an RE2 class for \P{name} under the i
// flag. ECMAScript takes the complement first, then matches a character when
// any of its case variants is in it, while RE2 folds the category first, then
// takes the complement. So the set holds every character with a case variant
// outside the category.
func foldedComplement(name string) string {
	if body, ok := foldedComplements.Load(name); ok {
		return body.(string)
	}
	table := unicode.Categories[name]
	body := classBody(func(r rune) bool {
		if !unicode.Is(table, r) {
			return true
		}
		for f := unicode.SimpleFold(r); f != r; f = unicode.SimpleFold(f) {
			if !unicode.Is(table, f) {
				return true
			}
		}
		return false
	})
	foldedComplements.Store(name, body)
	return body
}

// classBody returns the body of an RE2 class for the code points in, as
// ranges.
func classBody(in func(rune) bool) string {
	var b strings.Builder
	start := rune(-1) // where the current range starts; -1 outside one
	for r := rune(0); r <= unicode.MaxRune+1; r++ {
		switch inside := r <= unicode.MaxRune && in(r); {
		case inside && start < 0:
			start = r
		case !inside && start >= 0:
			fmt.Fprintf(&b, `\x{%x}-\x{%x}`, start, r-1)
			start = -1
		}
	}
	return b.String()
}
