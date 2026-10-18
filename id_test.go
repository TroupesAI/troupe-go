package troupe_test

import (
	"strings"
	"testing"

	"github.com/TroupesAI/troupe-go"
)

// SuggestID makes an ID of a name that fits the grammar, is no reserved word
// or ID, and is unique where its list's objects must be (Format 2.3).
func TestSuggestID(t *testing.T) {
	b, _, err := troupe.ReadBundle(strings.NewReader(`{"troupe":"0.14","kind":"bundle",
		"characters":[{"id":"kai","name":"Kai","description":"d"},{"id":"kai_2","name":"Kai","description":"d"},
			{"id":"` + strings.Repeat("x", 64) + `","name":"X","description":"d"}],
		"worlds":[{"id":"city","name":"City","locations":[{"id":"vault","name":"Vault"}]},{"id":"sea","name":"Sea"}],
		"scenarios":[{"id":"heist","variables":[{"id":"bell","type":"int","description":"d"}],"openings":[{"id":"o"}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ list, name, want string }{
		{"/characters", "Kai Moreau", "kai_moreau"},
		{"/characters", "  Élodie D'Arcy-Lune! ", "elodie_darcy_lune"},
		{"/characters", "Kai", "kai_3"},
		{"/characters", "City", "city_2"},
		{"/characters", "Bell", "bell_2"},
		{"/scenarios", "Bell", "bell"},
		{"/characters", "Narrator", "narrator_2"},
		{"/worlds", "True", "true_2"},
		{"/scenarios", "7th Seal", "scenario_7th_seal"},
		{"/characters", "李华", "character"},
		{"/characters", strings.Repeat("x", 70), strings.Repeat("x", 62) + "_2"},
		{"/scenarios/0/variables", "Sea", "sea_2"},
		{"/characters/0/variables", "Kai", "kai"},
		{"/characters/0/variables", "Scene", "scene_2"},
		{"/worlds/1/locations", "Vault", "vault_2"},
		{"/scenarios/0/openings", "O", "o_2"},
		{"/scenarios/0/entries", "O", "o"},
	} {
		if got, err := b.SuggestID(c.list, c.name); err != nil || got != c.want {
			t.Errorf("%s, %q: got %q, %v, want %q", c.list, c.name, got, err, c.want)
		}
	}
	for _, list := range []string{"/characters/0", "/characters/3/entries", "/worlds/0/openings", "/scenarios/0/locations", "/cast", "characters"} {
		if got, err := b.SuggestID(list, "Kai"); err == nil {
			t.Errorf("%s: got %q, and no error", list, got)
		}
	}
}
