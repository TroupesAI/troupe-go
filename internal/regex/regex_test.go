package regex

import "testing"

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
