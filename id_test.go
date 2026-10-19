package troupe_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/TroupesAI/troupe-go"
)

// CheckID accepts an ID that fits the grammar, is no reserved word or ID, and
// is unique where its list's objects must be (Format 2.3), and tells a taken
// ID apart.
func TestCheckID(t *testing.T) {
	b, _, err := troupe.ReadBundle(strings.NewReader(`{"troupe":"0.14","kind":"bundle",
		"characters":[{"id":"kai","name":"Kai","description":"d"}],
		"worlds":[{"id":"city","name":"City","locations":[{"id":"vault","name":"Vault"}]},{"id":"sea","name":"Sea"}],
		"scenarios":[{"id":"heist","variables":[{"id":"bell","type":"int","description":"d"}],"openings":[{"id":"o"}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		list, id string
		taken    bool
	}{
		{"/characters", "kai", true},
		{"/characters", "city", true},
		{"/characters", "heist", true},
		{"/characters", "bell", true},
		{"/scenarios/0/variables", "sea", true},
		{"/worlds/1/locations", "vault", true},
		{"/scenarios/0/openings", "o", true},
	} {
		if err := b.CheckID(c.list, c.id); !errors.Is(err, troupe.ErrIDTaken) {
			t.Errorf("%s, %s: %v", c.list, c.id, err)
		}
	}
	for _, c := range [][2]string{
		{"/characters", "kai_moreau"},
		{"/scenarios", "bell"},
		{"/characters/0/variables", "kai"},
		{"/scenarios/0/entries", "o"},
		{"/characters", strings.Repeat("x", 64)},
	} {
		if err := b.CheckID(c[0], c[1]); err != nil {
			t.Errorf("%s, %s: %v", c[0], c[1], err)
		}
	}
	for _, c := range [][2]string{
		{"/characters", "narrator"},
		{"/worlds", "true"},
		{"/characters/0/variables", "scene"},
		{"/characters", "Kai"},
		{"/characters", "7th"},
		{"/characters", strings.Repeat("x", 65)},
		{"/characters/0", "elena"},
		{"/characters/3/entries", "elena"},
		{"/worlds/0/openings", "elena"},
		{"/cast", "elena"},
	} {
		if err := b.CheckID(c[0], c[1]); err == nil || errors.Is(err, troupe.ErrIDTaken) {
			t.Errorf("%s, %s: %v", c[0], c[1], err)
		}
	}
}
