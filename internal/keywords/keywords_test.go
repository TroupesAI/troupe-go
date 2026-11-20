package keywords

import (
	"testing"
	"unicode"
)

// A regex keyword means what it means in ECMAScript with the u flag (Format
// 14), on the scanned text, whose every line break is \n (Format 7.2). Where
// RE2 reads a pattern differently, the translation gives ECMAScript's meaning:
// \s is Unicode white space, ſ and K are word characters under i, and \P{…}
// under i holds every character with a case variant outside the category. The
// expected results are V8's, on text whose line breaks are \n.
func TestMatchRegexKeyword(t *testing.T) {
	tests := []struct {
		keyword, text string
		want          bool
	}{
		{"/a.b/", "a\rb", false},
		{"/a.b/", "a\u2028b", false},
		{"/a.b/", "a\nb", false},
		{"/a.b/", "a\u0085b", true},
		{"/a.b/s", "a\rb", true},
		{"/a\\sb/", "a\vb", true},
		{"/a\\sb/", "a\u00a0b", true},
		{"/a\\sb/", "a\u3000b", true},
		{"/a\\sb/", "a\ufeffb", true},
		{"/a\\sb/", "a\u2028b", true},
		{"/a\\sb/", "a\u200bb", false},
		{"/a\\Sb/", "a\u00a0b", false},
		{"/a\\Sb/", "axb", true},
		{"/a[\\s]b/", "a\u3000b", true},
		{"/a[^\\s]b/", "a\u3000b", false},
		{"/a[^\\S]b/", "a\ufeffb", true},
		{"/\\w/i", "\u017f", true},
		{"/\\w/", "\u017f", false},
		{"/\\bs/i", "x\u017f", false},
		{"/\\bs/i", " \u017f", true},
		{"/x\\B/i", "x\u212a", true},
		{"/\\W/i", "\u017f", false},
		{"/[^\\w]/i", "\u212a", false},
		{"/k/i", "\u212a", true},
		{"/[^s]/i", "\u017f", false},
		{"/\\P{Lu}/i", "A", true},
		{"/[^\\P{Lu}]/i", "a", false},
		{"/[\\P{Lu}]/i", "A", true},
		{"/\\P{L}/i", "A", false},
		{"/\\P{Lu}/", "A", false},
		{"/^b/m", "a\nb", true},
		{"/a$/m", "a\nb", true},
		{"/^b/", "a\nb", false},
		{"/a$/", "a\nb", false},
		{"/a{2,3}/", "xaax", true},
		{"/a{2,3}/", "xax", false},
		{"/a{2,}b/", "aaab", true},
		{"/(?:ab)+c/", "ababc", true},
		{"/a+?b/", "aab", true},
		{"/cat|dog/", "hotdog", true},
		{"/\\/x/", "a/x", true},
		{"/[-a]/", "-", true},
		{"/[a-]/", "-", true},
		{"/[\\-]/", "-", true},
		{"/[^-a]/", "b", true},
		{"/\\./", "a", false},
		{"/[.]/", "a", false},
		{"/^b/m", "a\r\nb", true},
		{"/^b/m", "a\rb", true},
		{"/^b/m", "a\u2028b", true},
		{"/^b/m", "a\u2029b", true},
		{"/a$/m", "a\r\nb", true},
		{"/a$/m", "a\rb", true},
		{"/a$/m", "a\u2028b", true},
		{"/a$/m", "a\u2029b", true},
		{"/a.b/s", "a\r\nb", true},
		{"/a\\rb/", "a\rb", false},
		{"/a\\nb/", "a\rb", true},
	}
	for _, tt := range tests {
		k, err := Compile(tt.keyword)
		if err != nil {
			t.Errorf("%s: %v", tt.keyword, err)
			continue
		}
		if got := k.Match(NewText(tt.text)); got != tt.want {
			t.Errorf("%s on %q: %v, want %v", tt.keyword, tt.text, got, tt.want)
		}
	}
}

// A plain keyword that isn't one word, such as one holding a space or a
// hyphen, matches as a whole too, ignoring case, however case folding
// changes its length in bytes.
func TestMatchPlainKeyword(t *testing.T) {
	tests := []struct {
		keyword, text string
		want          bool
	}{
		{"storm lantern", "The STORM LANTERN, lit.", true},
		{"storm lantern", "two storm lanterns", false},
		{"storm lantern", "a brainstorm lantern", false},
		{"k-9", "\u212a-9 barks", true},
		{"k-9", "\u212a-90", false},
	}
	for _, tt := range tests {
		k, err := Compile(tt.keyword)
		if err != nil {
			t.Fatal(err)
		}
		if got := k.Match(NewText(tt.text)); got != tt.want {
			t.Errorf("%s on %q: %v, want %v", tt.keyword, tt.text, got, tt.want)
		}
	}
}

// A keyword that is one word matches the words of the folded text, which are
// those of the text only while no characters case folding relates are one a
// letter, mark or number and the other not. Unicode could break that.
func TestFoldKeepsWords(t *testing.T) {
	for r := range rune(unicode.MaxRune + 1) {
		for f := unicode.SimpleFold(r); f != r; f = unicode.SimpleFold(f) {
			if isWord(f) != isWord(r) {
				t.Errorf("%U folds with %U", r, f)
			}
		}
	}
}
