package troupe

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"strings"
)

// stored is a State as MarshalJSON writes it.
type stored struct {
	Troupe string `json:"troupe"`
	// Values holds every key the state holds, as All yields it, with its
	// value.
	Values    map[string]jsontext.Value `json:"values"`
	User      string                    `json:"user,omitempty"`
	Turn      int                       `json:"turn,omitzero"`
	Opening   string                    `json:"opening,omitempty"`
	At        string                    `json:"at,omitempty"`
	ParentAt  string                    `json:"parentAt,omitempty"`
	Entries   []Entry                   `json:"entries,omitempty"`
	Activated map[string]int            `json:"activated,omitempty"`
	// Appeared is always written, so a state stored before it is told
	// apart.
	Appeared *[]string `json:"appeared"`
}

// ErrStateFormat is what UnmarshalState's error wraps for a state stored in a
// format this release doesn't read, as another Troupe version's or an earlier
// release's may be. Apply computes it again.
var ErrStateFormat = errors.New("the stored state is in a format this release doesn't read")

// MarshalJSON returns s as JSON that UnmarshalState reads back: every key and
// value s holds, and what else it keeps that depends on the path, such as
// the rounds, the session entries in use, when entries were recorded as
// activated and who has been present. The JSON names this module's Troupe
// version; a later release reads it back or wraps ErrStateFormat. No string
// in it holds U+0000, so a database's JSON type that refuses it, such as
// PostgreSQL's jsonb, can hold it.
//
// It fails for the zero State.
func (s State) MarshalJSON() ([]byte, error) {
	if s.s == nil {
		return nil, errors.New("troupe: the zero State holds nothing to store")
	}
	v := stored{
		Troupe:    Version,
		Values:    map[string]jsontext.Value{},
		User:      s.s.user,
		Turn:      s.s.turn,
		Opening:   s.s.opening,
		At:        s.s.at,
		ParentAt:  s.s.parentAt,
		Entries:   s.s.entries,
		Activated: s.s.activated,
		Appeared:  &s.s.appeared,
	}
	for key, x := range s.All() {
		raw, err := json.Marshal(x)
		if err != nil {
			return nil, err
		}
		v.Values[key] = raw
	}
	data, err := json.Marshal(v, json.Deterministic(true))
	if err != nil {
		return nil, err
	}
	// JSON writes U+0000 as \u0000, and U+0001 as \u0001.
	if !bytes.Contains(data, []byte(`\u000`)) {
		return data, nil
	}
	return mapStrings(data, hideNUL.Replace)
}

// UnmarshalState returns the state that State.MarshalJSON wrote as data. It
// is this story's state as it was stored, but for what the story has added
// since: a key the stored state doesn't hold, such as one of a joiner the
// session's header has added, holds its initial value (Format 8.3).
//
// It fails when data isn't a stored state, or holds a key or value that isn't
// valid in this story (Format 8.5), and wraps ErrStateFormat when the state is
// stored in a format this release doesn't read.
func (st *Story) UnmarshalState(data []byte) (State, error) {
	if bytes.Contains(data, []byte(`\u0001`)) {
		var err error
		if data, err = mapStrings(data, showNUL.Replace); err != nil {
			return State{}, errors.New("troupe: the stored state isn't JSON")
		}
	}
	var v stored
	if json.Unmarshal(data, &v) != nil {
		return State{}, errors.New("troupe: the stored state isn't one State.MarshalJSON writes")
	}
	if v.Troupe != Version {
		return State{}, fmt.Errorf("troupe: %w: it is of another Troupe version", ErrStateFormat)
	}
	if v.Appeared == nil {
		return State{}, fmt.Errorf("troupe: %w: it doesn't say who has been present", ErrStateFormat)
	}
	sc := st.scope
	for key, raw := range v.Values {
		if !sc.valid(key, raw) {
			return State{}, errors.New("troupe: the stored state holds a key or value that isn't valid in this story")
		}
	}
	s := st.initial.clone()
	sc.apply(s, v.Values, nil)
	s.user, s.turn, s.opening, s.at, s.parentAt = v.User, v.Turn, v.Opening, v.At, v.ParentAt
	s.entries, s.activated, s.appeared = v.Entries, v.Activated, *v.Appeared
	return State{sc, s}, nil
}

// valid reports whether a change can set key to raw (Format 8.5). It doesn't
// check that a location or a character exists, which apply needs no more
// than a list of strings for.
func (sc *scope) valid(key string, raw jsontext.Value) bool {
	switch key {
	case "scene.location":
		return raw.Kind() == '"'
	case "scene.present":
		var ids []string
		return json.Unmarshal(raw, &ids) == nil
	}
	b, _, ok := sc.variable(key)
	return ok && validValue(b.v, raw)
}

// A stored state writes U+0000 as U+0001 0, and U+0001 as U+0001 U+0001.
var (
	hideNUL = strings.NewReplacer("\x01", "\x01\x01", "\x00", "\x010")
	showNUL = strings.NewReplacer("\x01\x01", "\x01", "\x010", "\x00")
)

// mapStrings returns the JSON value data with f applied to each of its
// strings, member names included.
func mapStrings(data []byte, f func(string) string) ([]byte, error) {
	dec := jsontext.NewDecoder(bytes.NewReader(data))
	var out bytes.Buffer
	enc := jsontext.NewEncoder(&out)
	for {
		tok, err := dec.ReadToken()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if tok.Kind() == '"' {
			tok = jsontext.String(f(tok.String()))
		}
		if err := enc.WriteToken(tok); err != nil {
			return nil, err
		}
	}
	return bytes.TrimSuffix(out.Bytes(), []byte("\n")), nil
}
