package regex

import "testing"

// A key means what it means in ECMAScript with the u flag (Format 14), where
// RE2 reads it differently: . excludes every line terminator, \s is Unicode
// white space, ſ and K are word characters under i, and \P{…} under i holds
// every character with a case variant outside the category. The expected
// results are V8's. The m flag at line terminators other than \n waits on the
// spec (docs/plan.md, spec feedback).
func TestCompileMeaning(t *testing.T) {
	tests := []struct {
		pattern, flags, text string
		want                 bool
	}{
		{"a.b", "", "a\rb", false},
		{"a.b", "", "a\u2028b", false},
		{"a.b", "", "a\nb", false},
		{"a.b", "", "a\u0085b", true},
		{"a.b", "s", "a\rb", true},
		{"a\\sb", "", "a\vb", true},
		{"a\\sb", "", "a\u00a0b", true},
		{"a\\sb", "", "a\u3000b", true},
		{"a\\sb", "", "a\ufeffb", true},
		{"a\\sb", "", "a\u2028b", true},
		{"a\\sb", "", "a\u200bb", false},
		{"a\\Sb", "", "a\u00a0b", false},
		{"a\\Sb", "", "axb", true},
		{"a[\\s]b", "", "a\u3000b", true},
		{"a[^\\s]b", "", "a\u3000b", false},
		{"a[^\\S]b", "", "a\ufeffb", true},
		{"\\w", "i", "\u017f", true},
		{"\\w", "", "\u017f", false},
		{"\\bs", "i", "x\u017f", false},
		{"\\bs", "i", " \u017f", true},
		{"x\\B", "i", "x\u212a", true},
		{"\\W", "i", "\u017f", false},
		{"[^\\w]", "i", "\u212a", false},
		{"k", "i", "\u212a", true},
		{"[^s]", "i", "\u017f", false},
		{"\\P{Lu}", "i", "A", true},
		{"[^\\P{Lu}]", "i", "a", false},
		{"[\\P{Lu}]", "i", "A", true},
		{"\\P{L}", "i", "A", false},
		{"\\P{Lu}", "", "A", false},
		{"^b", "m", "a\nb", true},
		{"a$", "m", "a\nb", true},
		{"^b", "", "a\nb", false},
		{"a$", "", "a\nb", false},
		{"a{2,3}", "", "xaax", true},
		{"a{2,3}", "", "xax", false},
		{"a{2,}b", "", "aaab", true},
		{"(?:ab)+c", "", "ababc", true},
		{"a+?b", "", "aab", true},
		{"cat|dog", "", "hotdog", true},
		{"\\/x", "", "a/x", true},
		{"[-a]", "", "-", true},
		{"[a-]", "", "-", true},
		{"[\\-]", "", "-", true},
		{"[^-a]", "", "b", true},
		{"\\.", "", "a", false},
		{"[.]", "", "a", false},
	}
	for _, tt := range tests {
		re, err := Compile(tt.pattern, tt.flags)
		if err != nil {
			t.Errorf("/%s/%s: %v", tt.pattern, tt.flags, err)
			continue
		}
		if got := re.MatchString(tt.text); got != tt.want {
			t.Errorf("/%s/%s on %q: %v, want %v", tt.pattern, tt.flags, tt.text, got, tt.want)
		}
	}
}

// The grammar of Format 14, in the forms the conformance suite has no case
// for.
func TestCompileGrammar(t *testing.T) {
	tests := []struct {
		pattern string
		valid   bool
	}{
		{`a/b`, false},     // / stands for itself only escaped
		{`a}`, false},      // and so does }
		{`a{x}`, false},    // and { outside a count
		{`a{,2}`, false},   // a count has its lower bound
		{`a{2,1}`, false},  // n ≤ m
		{`a{0,0}`, false},  // no {0,0}
		{`a{0,}`, true},    // but {0,} and lazy counts are fine
		{`a{2}?`, true},    //
		{`a**`, false},     // a quantifier follows an atom
		{`a{1001}`, false}, // 1001 bytes written out
		{`\-`, false},      // \- only in a class
		{`[\d-z]`, false},  // a range is between characters
		{`[^]`, false},     // a class holds something
		{`[\b]`, false},    // \b isn't a class escape
		{`\u0041`, false},  // no escapes but those listed
		{`\p{LC}`, false},  // no LC
		{`(?<n>a)`, false}, // only (…) and (?:…)
		{`(a`, false},      //
		{`a)`, false},      //
	}
	for _, tt := range tests {
		if _, err := Compile(tt.pattern, ""); (err == nil) != tt.valid {
			t.Errorf("/%s/: %v, want valid %v", tt.pattern, err, tt.valid)
		}
	}
}
