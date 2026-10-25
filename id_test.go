package troupe_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/TroupesAI/troupe-go"
)

// CheckID accepts an ID that fits the grammar, is no reserved word or ID, and
// is unique where its list's objects must be (Format 2.3), and tells a bad
// pointer, an invalid ID and a taken one apart.
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
		want     error
	}{
		{"/characters", "kai_moreau", nil},
		{"/scenarios", "bell", nil},
		{"/characters/0/variables", "kai", nil},
		{"/scenarios/0/entries", "o", nil},
		{"/characters", strings.Repeat("x", 64), nil},
		{"/characters", "kai", troupe.ErrIDTaken},
		{"/characters", "city", troupe.ErrIDTaken},
		{"/characters", "heist", troupe.ErrIDTaken},
		{"/characters", "bell", troupe.ErrIDTaken},
		{"/scenarios/0/variables", "sea", troupe.ErrIDTaken},
		{"/worlds/1/locations", "vault", troupe.ErrIDTaken},
		{"/scenarios/0/openings", "o", troupe.ErrIDTaken},
		{"/characters", "narrator", troupe.ErrIDInvalid},
		{"/worlds", "true", troupe.ErrIDInvalid},
		{"/characters/0/variables", "scene", troupe.ErrIDInvalid},
		{"/characters", "Kai", troupe.ErrIDInvalid},
		{"/characters", "7th", troupe.ErrIDInvalid},
		{"/characters", strings.Repeat("x", 65), troupe.ErrIDInvalid},
		{"/characters/0", "elena", troupe.ErrBadPointer},
		{"/characters/3/entries", "elena", troupe.ErrBadPointer},
		{"/worlds/0/openings", "elena", troupe.ErrBadPointer},
		{"/cast", "elena", troupe.ErrBadPointer},
	} {
		if err := b.CheckID(c.list, c.id); !errors.Is(err, c.want) {
			t.Errorf("%s, %s: %v", c.list, c.id, err)
		}
	}
}
