package troupe_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/TroupesAI/troupe-go"
)

// own means the variables of the object a text or condition belongs to
// (Format 8.2), even when another owner shares its ID, so Check reports only
// the DUPLICATE_ID.
func TestCheckOwnSharedID(t *testing.T) {
	for _, c := range []struct{ name, bundle, at string }{
		{"scenario", `"characters":[{"id":"kai","name":"Kai","description":"{{own.nerve}}",
			"variables":[{"id":"nerve","type":"int","description":"n"}],
			"entries":[{"id":"e","when":"own.nerve >= 2","text":"t"}]}],
			"scenarios":[{"id":"kai","variables":[{"id":"bell","type":"int","description":"b"}],
			"entries":[{"id":"p","when":"own.bell > 0","text":"t"}]}]`, "/scenarios/0/id"},
		{"character", `"characters":[{"id":"kai","name":"Kai","description":"d"},
			{"id":"kai","name":"Kai","description":"{{own.nerve}}",
			"variables":[{"id":"nerve","type":"int","description":"n"}],
			"entries":[{"id":"e","when":"own.nerve >= 2","text":"t"}]}],
			"scenarios":[{"id":"s"}]`, "/characters/1/id"},
	} {
		t.Run(c.name, func(t *testing.T) {
			b, diags, err := troupe.ReadBundle(strings.NewReader(`{"troupe":"0.14","kind":"bundle",` + c.bundle + `}`))
			if err != nil || diags != nil {
				t.Fatal(err, diags)
			}
			want := []troupe.Diagnostic{{Code: troupe.CodeDuplicateID, Pointer: c.at}}
			if got := troupe.Check(b, nil); !slices.Equal(got, want) {
				t.Errorf("Check = %v, want %v", got, want)
			}
		})
	}
}
