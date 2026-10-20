package troupe_test

import (
	"bytes"
	"encoding/json/v2"
	"io/fs"
	"strings"
	"testing"

	"github.com/TroupesAI/troupe-go"
)

// The model marshals as the spec writes it, so a conforming document read
// and marshaled again is the same JSON, every ext included: the spec's
// examples here. A bundle marshals as the same bytes every time.
func TestMarshalModel(t *testing.T) {
	for _, name := range []string{"elena-card.troupe.json", "mira-card.troupe.json", "lantern-heist/bundle.json"} {
		data, err := fs.ReadFile(spec, "examples/"+name)
		if err != nil {
			t.Fatal(err)
		}
		b, diags, err := troupe.ReadPackageBundle(bytes.NewReader(data), []string{"cover.webp", "elena.webp", "kai.webp"})
		if err != nil || diags != nil {
			t.Fatal(name, err, diags)
		}
		if same, err := sameJSON(b, data); err != nil || !same {
			t.Errorf("%s marshals as different JSON: %v", name, err)
		}
		first, err := json.Marshal(b)
		if err != nil {
			t.Fatal(err)
		}
		for range 20 {
			if again, _ := json.Marshal(b); !bytes.Equal(again, first) {
				t.Errorf("%s marshals as different bytes", name)
				break
			}
		}
	}

	data, err := fs.ReadFile(spec, "examples/lantern-heist/sessions/heist.troupe-session.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	s, diags, err := troupe.ReadSession(bytes.NewReader(data))
	if err != nil || diags != nil {
		t.Fatal(err, diags)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	values := []any{s.Header}
	for _, m := range s.Messages {
		values = append(values, m)
	}
	for i, v := range values {
		if same, err := sameJSON(v, []byte(lines[i])); err != nil || !same {
			out, _ := json.Marshal(v)
			t.Errorf("line %d marshals as %s: %v", i+1, out, err)
		}
	}
}
