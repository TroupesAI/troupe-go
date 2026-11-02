// Package keywords matches entry keywords against text (Format 7.2).
package keywords

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/TroupesAI/troupe-go/internal/regex"
)

// regexKeyword is the form of a regex keyword, /pattern/flags: its pattern is
// the text between the first and the last /, line breaks included, and its
// flags the letters after (Format 7.2).
var regexKeyword = regexp.MustCompile(`(?s)^/(.+)/([a-z]*)$`)

// anywhere are the scripts whose keywords match anywhere, not only as words.
var anywhere = []*unicode.RangeTable{
	unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul,
	unicode.Thai, unicode.Lao, unicode.Khmer, unicode.Myanmar, unicode.Tibetan,
}

// lineBreaks writes every line break as \n.
var lineBreaks = strings.NewReplacer("\r\n", "\n", "\r", "\n", "\u2028", "\n", "\u2029", "\n")

// Text returns s as keywords match it, with every line break, \r\n, a lone \r,
// U+2028 and U+2029, written as \n (Format 7.2). The scanned text is
// converted once, before any keyword is matched against it.
func Text(s string) string {
	return lineBreaks.Replace(s)
}

// Keyword is a compiled keyword.
type Keyword struct {
	matcher interface{ MatchString(string) bool } // nil for the empty keyword
}

// Compile compiles keyword. It fails only for a regex keyword that isn't valid
// (Format 14).
func Compile(keyword string) (*Keyword, error) {
	if m := regexKeyword.FindStringSubmatch(keyword); m != nil {
		re, err := regex.Compile(m[1], m[2])
		if err != nil {
			return nil, err
		}
		return &Keyword{re}, nil
	}
	if keyword == "" {
		return &Keyword{}, nil
	}
	// A plain keyword matches ignoring case, by simple case folding, as RE2's
	// (?i) does: as a whole word, with no letter, mark or number right
	// before or after it, unless it has a character of a script that
	// matches anywhere.
	pattern := "(?i)" + regexp.QuoteMeta(keyword)
	if strings.IndexFunc(keyword, func(r rune) bool { return unicode.In(r, anywhere...) }) < 0 {
		pattern = `(?i)(?:^|[^\pL\pM\pN])` + regexp.QuoteMeta(keyword) + `(?:$|[^\pL\pM\pN])`
	}
	return &Keyword{regexp.MustCompile(pattern)}, nil
}

// Match reports whether k matches anywhere in text, as Text returns it. An
// empty plain keyword matches nothing.
func (k *Keyword) Match(text string) bool {
	return k.matcher != nil && k.matcher.MatchString(text)
}
