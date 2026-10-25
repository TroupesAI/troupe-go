package troupe_test

import (
	"fmt"
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

// The "*" map limit is judged on each path: a sibling's changes are undone
// before the next sibling's are judged, its keys removed and those it removed
// put back.
func TestCheckSiblingPaths(t *testing.T) {
	b, diags, err := troupe.ReadBundle(strings.NewReader(`{"troupe":"0.14","kind":"bundle",
		"scenarios":[{"id":"s","variables":[{"id":"inv","type":"int","of":"*","description":"d"}]}]}`))
	if err != nil || diags != nil {
		t.Fatal(err, diags)
	}
	// The root leaves the map one key short of the limit.
	keys := make([]string, 255)
	for i := range keys {
		keys[i] = fmt.Sprintf(`"state.inv.k%d":1`, i)
	}
	// Of the siblings, each removing a key, only the middle one adds enough
	// to pass the limit, so whichever order they're judged in, a key not put
	// back or one not removed changes what is reported.
	msg := func(id, set string) string {
		return `{"id":"` + id + `","parent":"r","from":"player","content":[{"text":"x"}],"changes":[{"by":"player","set":{` + set + `}}]}`
	}
	s, diags, err := troupe.ReadSession(strings.NewReader(strings.Join([]string{
		`{"troupe":"0.14","kind":"session","scenario":"s"}`,
		`{"id":"r","from":"player","content":[{"text":"x"}],"changes":[{"by":"player","set":{` + strings.Join(keys, ",") + `}}]}`,
		msg("a", `"state.inv.k1":0,"state.inv.n4":1,"state.inv.n5":1`),
		msg("b", `"state.inv.k0":0,"state.inv.n1":1,"state.inv.n2":1,"state.inv.n3":1`),
		msg("c", `"state.inv.k2":0,"state.inv.n6":1,"state.inv.n7":1`),
	}, "\n")))
	if err != nil || diags != nil {
		t.Fatal(err, diags)
	}
	var want []troupe.Diagnostic
	for _, key := range []string{"k0", "n1", "n2", "n3"} {
		want = append(want, troupe.Diagnostic{Code: troupe.CodeInvalidValue, Line: 4, Message: "b", Pointer: "/changes/0/set/state.inv." + key})
	}
	if got := troupe.Check(b, s); !slices.Equal(got, want) {
		t.Errorf("Check = %v, want %v", got, want)
	}
}
