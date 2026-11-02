package troupe_test

import (
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/TroupesAI/troupe-go"
)

// A pick's option comes from the caller's random source, which chooses every
// option in turn.
func TestRenderPick(t *testing.T) {
	st, _, err := troupe.ReadStory(strings.NewReader(`{"troupe":"0.15","kind":"story","scenarios":[{"id":"s"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	play, err := troupe.NewPlay(st, nil)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	r := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // G404: a seeded source makes the test reproducible
	for range 100 {
		seen[play.Render("{{pick::a::b::c}}", troupe.RenderContext{State: play.Initial()}, r)] = true
	}
	if len(seen) != 3 || !seen["a"] || !seen["b"] || !seen["c"] {
		t.Errorf("a source chose %v, want each option", seen)
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
