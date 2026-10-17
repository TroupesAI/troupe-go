// Package keys matches entry keys against text (Format 7.2).
package keys

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/TroupesAI/troupe-go/internal/regex"
)

// regexKey is the form of a regex key, /pattern/flags: its pattern is the
// text between the first and the last /, and its flags the letters after.
var regexKey = regexp.MustCompile(`^/(.+)/([a-z]*)$`)

// anywhere are the scripts whose keys match anywhere, not only as words.
var anywhere = []*unicode.RangeTable{
	unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul,
	unicode.Thai, unicode.Lao, unicode.Khmer, unicode.Myanmar, unicode.Tibetan,
}

// Key is a compiled key.
type Key struct {
	matcher interface{ MatchString(string) bool } // nil for the empty key
}

// Compile compiles key. It fails only for a regex key that isn't valid
// (Format 14).
func Compile(key string) (*Key, error) {
	if m := regexKey.FindStringSubmatch(key); m != nil {
		re, err := regex.Compile(m[1], m[2])
		if err != nil {
			return nil, err
		}
		return &Key{re}, nil
	}
	if key == "" {
		return &Key{}, nil
	}
	// A plain key matches ignoring case, by simple case folding, as RE2's
	// (?i) does: as a whole word, with no letter, mark or number right
	// before or after it, unless it has a character of a script that
	// matches anywhere.
	pattern := "(?i)" + regexp.QuoteMeta(key)
	if strings.IndexFunc(key, func(r rune) bool { return unicode.In(r, anywhere...) }) < 0 {
		pattern = `(?i)(?:^|[^\pL\pM\pN])` + regexp.QuoteMeta(key) + `(?:$|[^\pL\pM\pN])`
	}
	return &Key{regexp.MustCompile(pattern)}, nil
}

// Match reports whether k matches anywhere in text. An empty plain key
// matches nothing.
func (k *Key) Match(text string) bool {
	return k.matcher != nil && k.matcher.MatchString(text)
}
