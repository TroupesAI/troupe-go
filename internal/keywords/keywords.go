// Package keywords matches entry keywords against text (Format 7.2).
package keywords

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

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

// Text is scanned text as keywords match it.
type Text struct {
	text   string // every line break, \r\n, a lone \r, U+2028 and U+2029, written as \n (Format 7.2)
	folded string // text, case folded
	// words are the words of folded: its longest runs of letters, marks and
	// numbers. No characters that case folding relates are one a letter,
	// mark or number and the other not, so these are the words of text,
	// folded.
	words map[string]bool
}

// NewText returns s as keywords match it. Scanned text is converted once,
// before any keyword is matched against it.
func NewText(s string) Text {
	t := Text{text: lineBreaks.Replace(s), words: map[string]bool{}}
	t.folded = fold(t.text)
	for w := range strings.FieldsFuncSeq(t.folded, func(r rune) bool { return !isWord(r) }) {
		t.words[w] = true
	}
	return t
}

// Keyword is a compiled keyword.
type Keyword struct {
	re *regex.Regexp // a regex keyword
	// A plain keyword, case folded. One that is a word, only letters, marks
	// and numbers, matches a word of the text. The empty keyword is one, and
	// no word is empty.
	folded         string
	word, anywhere bool
}

// Compile compiles keyword. It fails only for a regex keyword that isn't valid
// (Format 14).
func Compile(keyword string) (*Keyword, error) {
	if m := regexKeyword.FindStringSubmatch(keyword); m != nil {
		re, err := regex.Compile(m[1], m[2])
		if err != nil {
			return nil, err
		}
		return &Keyword{re: re}, nil
	}
	// A plain keyword matches ignoring case, by simple case folding: as a
	// whole word, with no letter, mark or number right before or after it,
	// unless it has a character of a script that matches anywhere.
	k := &Keyword{folded: fold(keyword)}
	k.anywhere = strings.IndexFunc(keyword, func(r rune) bool { return unicode.In(r, anywhere...) }) >= 0
	k.word = !k.anywhere && strings.IndexFunc(keyword, func(r rune) bool { return !isWord(r) }) < 0
	return k, nil
}

// Match reports whether k matches anywhere in t. An empty plain keyword
// matches nothing.
func (k *Keyword) Match(t Text) bool {
	switch {
	case k.re != nil:
		return k.re.MatchString(t.text)
	case k.word:
		return t.words[k.folded]
	case k.anywhere:
		return strings.Contains(t.folded, k.folded)
	}
	for i := 0; ; {
		j := strings.Index(t.folded[i:], k.folded)
		if j < 0 {
			return false
		}
		start, end := i+j, i+j+len(k.folded)
		before, _ := utf8.DecodeLastRuneInString(t.folded[:start])
		after, _ := utf8.DecodeRuneInString(t.folded[end:])
		if (start == 0 || !isWord(before)) && (end == len(t.folded) || !isWord(after)) {
			return true
		}
		_, n := utf8.DecodeRuneInString(t.folded[start:])
		i = start + n
	}
}

// fold returns s with each character as the least of those simple case
// folding relates it to, so two strings fold alike exactly when they are
// equal ignoring case (Format 7.2).
func fold(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		least := r
		for f := unicode.SimpleFold(r); f != r; f = unicode.SimpleFold(f) {
			least = min(least, f)
		}
		b.WriteRune(least)
	}
	return b.String()
}

func isWord(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsMark(r) || unicode.IsNumber(r)
}
