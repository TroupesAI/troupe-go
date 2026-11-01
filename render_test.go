package troupe_test

import (
	"maps"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/TroupesAI/troupe-go"
)

// A pick's option comes from the caller's random source, which chooses every
// option in turn. Options split at each :: from the left (Format 9).
func TestRenderPick(t *testing.T) {
	st, _, err := troupe.ReadStory(strings.NewReader(`{"troupe":"0.15","kind":"story","scenarios":[{"id":"s"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	play, err := troupe.NewPlay(st, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // G404: a seeded source makes the test reproducible
	for text, want := range map[string][]string{
		"{{pick::a::b::c}}": {"a", "b", "c"},
		"{{pick::a:::b}}":   {":b", "a"},
		"{{pick:::a::b:}}":  {":a", "b:"},
	} {
		seen := map[string]bool{}
		for range 100 {
			seen[play.Render(text, troupe.RenderContext{State: play.Initial()}, r)] = true
		}
		if got := slices.Sorted(maps.Keys(seen)); !slices.Equal(got, want) {
			t.Errorf("%s: a source chose %q, want each of %q", text, got, want)
		}
	}
}

// Escaped text resolves to exactly what was written.
func TestEscape(t *testing.T) {
	st, _, err := troupe.ReadStory(strings.NewReader(`{"troupe":"0.15","kind":"story",
		"characters":[{"id":"a","name":"Ann","description":"Ann."}],"scenarios":[{"id":"s"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	play, err := troupe.NewPlay(st, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // G404: a seeded source makes the test reproducible
	c := troupe.RenderContext{State: play.Initial(), Char: "a"}
	for _, text := range []string{`\`, `\\{{`, `{{char}}`, `{{{char}}`, `\{{char}}`, `{{pick::x::y}}`, `a}}{\`} {
		if got := play.Render(troupe.Escape(text), c, r); got != text {
			t.Errorf("Escape(%q) renders as %q", text, got)
		}
	}
}

// A state path is author IDs joined by dots, so a segment that is a CEL
// reserved word or longer than 64 bytes makes no macro: the text is left as
// written and is UNKNOWN_MACRO, though the variable is declared (Format 2.3,
// 9).
func TestRenderPathSegments(t *testing.T) {
	for _, key := range []string{"in", strings.Repeat("k", 65)} {
		text := "{{state.trust." + key + "}}"
		st, _, err := troupe.ReadStory(strings.NewReader(`{"troupe":"0.15","kind":"story",
			"scenarios":[{"id":"s","premise":"` + text + `","variables":[{"id":"trust","type":"int","per":"*","description":"d"}]}]}`))
		if err != nil {
			t.Fatal(err)
		}
		want := []troupe.Diagnostic{{Code: troupe.CodeUnknownMacro, Pointer: "/scenarios/0/premise"}}
		if got := troupe.Check(st, nil); !slices.Equal(got, want) {
			t.Errorf("%s: diagnostics %v, want %v", text, got, want)
		}
		play, err := troupe.NewPlay(st, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got := play.Render(text, troupe.RenderContext{State: play.Initial()}, nil); got != text {
			t.Errorf("%s renders as %q", text, got)
		}
	}
}
