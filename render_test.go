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
	b, _, err := troupe.ReadBundle(strings.NewReader(`{"troupe":"0.14","kind":"bundle","scenarios":[{"id":"s"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	story, err := troupe.NewStory(b, nil)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	r := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // G404: a seeded source makes the test reproducible
	for range 100 {
		seen[story.Render("{{pick::a::b::c}}", troupe.RenderContext{State: story.Initial()}, r)] = true
	}
	if len(seen) != 3 || !seen["a"] || !seen["b"] || !seen["c"] {
		t.Errorf("a source chose %v, want each option", seen)
	}
}
