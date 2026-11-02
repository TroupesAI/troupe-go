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
// examples, and documents setting the optional fields they leave out. A story
// marshals as the same bytes every time.
func TestMarshalModel(t *testing.T) {
	stories := map[string][]byte{"every field": []byte(`{"$schema":"https://x.test/s.json","troupe":"0.15","kind":"story","ext":{"a.b":1},
		"characters":[{"id":"kai","name":"Kai","description":"d","examples":[[{"speaker":"player","text":"Hi.","ext":{"a.b":2}}]],"ext":{"a.b":3}}],
		"worlds":[{"id":"w","name":"W","locations":[{"id":"l","name":"L","image":"https://x.test/l.png","ext":{"a.b":4}}],
			"variables":[{"id":"rain","type":"bool","description":"d"}],"ext":{"a.b":5}}],
		"scenarios":[{"id":"s","name":"S","tagline":"T","mode":"narrator","narratorName":"the Chorus","worlds":["w"],
			"cast":[{"character":"kai","player":true,"spoiler":true,"ext":{"a.b":6}}],
			"openings":[{"id":"o","name":"O","content":[{"text":"x","knownBy":["kai"],"ext":{"a.b":7}},
				{"type":"image","src":"https://x.test/o.png","alt":"A","ext":{"a.b":12}}],"ext":{"a.b":8}}],
			"entries":[{"id":"e","text":"t","title":"E","retrievable":true,"scanDepth":2,"once":true,"disabled":true,"ext":{"a.b":9}}],
			"variables":[{"id":"v","type":"bool","description":"d","ext":{"a.b":10}}],"ext":{"a.b":11}},
			{"id":"empty","worlds":[]}]}`)}
	for _, name := range []string{"elena.troupe.json", "mira.troupe.json", "lantern-heist/story.json"} {
		data, err := fs.ReadFile(spec, "examples/"+name)
		if err != nil {
			t.Fatal(err)
		}
		stories[name] = data
	}
	for name, data := range stories {
		st, diags, err := troupe.ReadPackageStory(bytes.NewReader(data), []string{"cover.webp", "elena.webp", "kai.webp"})
		if err != nil || diags != nil {
			t.Fatal(name, err, diags)
		}
		if same, err := sameJSON(st, data); err != nil || !same {
			t.Errorf("%s marshals as different JSON: %v", name, err)
		}
		first, err := json.Marshal(st)
		if err != nil {
			t.Fatal(err)
		}
		for range 20 {
			if again, _ := json.Marshal(st); !bytes.Equal(again, first) {
				t.Errorf("%s marshals as different bytes", name)
				break
			}
		}
	}

	example, err := fs.ReadFile(spec, "examples/lantern-heist/sessions/heist.troupe-session.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{
		"the example session": example,
		"every field": []byte(`{"$schema":"https://x.test/h.json","troupe":"0.15","kind":"session","name":"N","story":"abc","boundaries":{"lines":["a"],"veils":["b"],"ext":{"a.b":1}},"scenario":"s","worlds":[{"id":"x","name":"X"}],"ext":{"a.b":2}}
{"id":"1","from":"player","content":[{"text":"x","ext":{"a.b":3}}],"changes":[{"by":"player","set":{"state.v":true},"ext":{"a.b":4}}],"hidden":true,"ext":{"a.b":5}}`),
	} {
		s, diags, err := troupe.ReadSession(bytes.NewReader(data))
		if err != nil || diags != nil {
			t.Fatal(name, err, diags)
		}
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		values := []any{s.Header}
		for _, m := range s.Messages {
			values = append(values, m)
		}
		for i, v := range values {
			if same, err := sameJSON(v, []byte(lines[i])); err != nil || !same {
				out, _ := json.Marshal(v)
				t.Errorf("%s, line %d marshals as %s: %v", name, i+1, out, err)
			}
		}
	}
}
