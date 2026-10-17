package keys

import "testing"

// A regex key means what it means in ECMAScript with the u flag (Format 14),
// on the scanned text, whose every line break is \n (Format 7.2). Where RE2
// reads a pattern differently, the translation gives ECMAScript's meaning:
// \s is Unicode white space, ſ and K are word characters under i, and \P{…}
// under i holds every character with a case variant outside the category.
// The expected results are V8's, on text whose line breaks are \n.
func TestMatchRegexKey(t *testing.T) {
	tests := []struct {
		key, text string
		want      bool
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
		k, err := Compile(tt.key)
		if err != nil {
			t.Errorf("%s: %v", tt.key, err)
			continue
		}
		if got := k.Match(Text(tt.text)); got != tt.want {
			t.Errorf("%s on %q: %v, want %v", tt.key, tt.text, got, tt.want)
		}
	}
}
