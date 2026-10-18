package troupe

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
)

// Bundle is a bundle (Format 3).
type Bundle struct {
	Schema     string         `json:"$schema,omitempty"`
	Troupe     string         `json:"troupe"`
	ID         string         `json:"id,omitempty"`
	Name       string         `json:"name,omitempty"`
	Tagline    string         `json:"tagline,omitempty"`
	Version    string         `json:"version,omitempty"`
	Authors    []string       `json:"authors,omitempty"`
	Credits    string         `json:"credits,omitempty"`
	Language   string         `json:"language,omitempty"`
	Tags       []string       `json:"tags,omitempty"`
	Rating     string         `json:"rating,omitempty"` // "general", "mature" or "explicit"
	Warnings   []string       `json:"warnings,omitempty"`
	Cover      string         `json:"cover,omitempty"`
	Characters []Character    `json:"characters,omitempty"`
	Worlds     []World        `json:"worlds,omitempty"`
	Scenarios  []Scenario     `json:"scenarios,omitempty"`
	Ext        jsontext.Value `json:"ext,omitzero"`
}

// Character is a character (Format 4).
type Character struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Aliases     []string       `json:"aliases,omitempty"`
	Description string         `json:"description"`
	Summary     string         `json:"summary,omitempty"`
	Examples    [][]Line       `json:"examples,omitempty"`
	Entries     []Entry        `json:"entries,omitempty"`
	Variables   []Variable     `json:"variables,omitempty"`
	Avatar      string         `json:"avatar,omitempty"`
	Ext         jsontext.Value `json:"ext,omitzero"`
}

// Line is a line of a character's example exchange (Format 4).
type Line struct {
	Speaker string         `json:"speaker,omitempty"` // "char", "user" or a character ID; "" for narration
	Text    string         `json:"text"`
	Ext     jsontext.Value `json:"ext,omitzero"`
}

// World is a world (Format 5).
type World struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Locations   []Location     `json:"locations,omitempty"`
	Entries     []Entry        `json:"entries,omitempty"`
	Variables   []Variable     `json:"variables,omitempty"`
	Ext         jsontext.Value `json:"ext,omitzero"`
}

// Location is a place in a world (Format 5).
type Location struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parent      string         `json:"parent,omitempty"`
	Image       string         `json:"image,omitempty"`
	Ext         jsontext.Value `json:"ext,omitzero"`
}

// Scenario is a scenario (Format 6).
type Scenario struct {
	ID       string       `json:"id"`
	Name     string       `json:"name,omitempty"`
	Tagline  string       `json:"tagline,omitempty"`
	Premise  string       `json:"premise,omitempty"`
	Briefing string       `json:"briefing,omitempty"`
	Mode     string       `json:"mode,omitempty"`     // "characters" or "narrator"; "" for characters
	Narrator string       `json:"narrator,omitempty"` // "" for "the narrator"
	Cast     []CastMember `json:"cast,omitempty"`
	// Worlds is nil when absent, which means every bundle world, and points to
	// an empty list for none.
	Worlds    *[]string      `json:"worlds,omitzero"`
	Openings  []Opening      `json:"openings,omitempty"`
	Entries   []Entry        `json:"entries,omitempty"`
	Variables []Variable     `json:"variables,omitempty"`
	Ext       jsontext.Value `json:"ext,omitzero"`
}

// CastMember is a member of a scenario's cast (Format 6.1).
type CastMember struct {
	Character string         `json:"character,omitempty"` // "" for a seat
	Player    bool           `json:"player,omitzero"`
	Notes     string         `json:"notes,omitempty"`
	Spoiler   bool           `json:"spoiler,omitzero"`
	Ext       jsontext.Value `json:"ext,omitzero"`
}

// Opening is a way a scenario starts (Format 6.3).
type Opening struct {
	ID      string                    `json:"id"`
	Title   string                    `json:"title,omitempty"`
	Set     map[string]jsontext.Value `json:"set,omitempty"`
	Content []Part                    `json:"content,omitempty"`
	Ext     jsontext.Value            `json:"ext,omitzero"`
}

// Entry is an entry (Format 7).
type Entry struct {
	ID        string         `json:"id"`
	Text      string         `json:"text"`
	Title     string         `json:"title,omitempty"`
	Keys      []string       `json:"keys,omitempty"`
	Pull      bool           `json:"pull,omitzero"`
	When      string         `json:"when,omitempty"`
	ScanDepth *int           `json:"scanDepth,omitzero"` // nil for 4
	Once      bool           `json:"once,omitzero"`
	Sticky    int64          `json:"sticky,omitzero"`   // 0 for none
	Cooldown  int64          `json:"cooldown,omitzero"` // 0 for none
	Chance    *int           `json:"chance,omitzero"`   // a percent; nil for 100
	Group     string         `json:"group,omitempty"`
	Weight    int            `json:"weight,omitzero"`    // 0 for 1
	Priority  *int           `json:"priority,omitzero"`  // nil for 50
	Position  string         `json:"position,omitempty"` // "system" or "context"; "" for context, unless Depth is set
	Depth     *int64         `json:"depth,omitzero"`
	KnownBy   []string       `json:"knownBy,omitempty"` // nil for everyone
	Disabled  bool           `json:"disabled,omitzero"`
	Ext       jsontext.Value `json:"ext,omitzero"`
}

// Variable is a variable's declaration (Format 8).
type Variable struct {
	ID          string            `json:"id"`
	Type        string            `json:"type"` // "bool", "int", "string", "enum" or "set"
	Values      []string          `json:"values,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Min         *int64            `json:"min,omitzero"`
	Max         *int64            `json:"max,omitzero"`
	Initial     jsontext.Value    `json:"initial,omitzero"` // nil for the type's default
	Of          Keys              `json:"of,omitzero"`
	Description string            `json:"description"`
	By          []string          `json:"by,omitempty"` // "player" and "model"
	MaxStep     int64             `json:"maxStep,omitzero"`
	OneWay      bool              `json:"oneWay,omitzero"`
	Spoiler     bool              `json:"spoiler,omitzero"`
	Ext         jsontext.Value    `json:"ext,omitzero"`
}

// Keys is what a keyed variable has one value for (Format 8.3). Its zero
// value means the variable isn't keyed.
type Keys struct {
	Cast bool     // "cast"
	Any  bool     // "*"
	List []string // the keys listed
}

// MarshalJSON writes k as of is written: "cast", "*" or the list.
func (k Keys) MarshalJSON() ([]byte, error) {
	switch {
	case k.Cast:
		return []byte(`"cast"`), nil
	case k.Any:
		return []byte(`"*"`), nil
	}
	return json.Marshal(k.List)
}

// UnmarshalJSON reads k as of is written.
func (k *Keys) UnmarshalJSON(data []byte) error {
	var s string
	if json.Unmarshal(data, &s) == nil {
		*k = Keys{Cast: s == "cast", Any: s == "*"}
		return nil
	}
	*k = Keys{}
	return json.Unmarshal(data, &k.List)
}

// Session is a session: its header line, then its messages (Format 11).
type Session struct {
	Header   SessionHeader
	Messages []Message // in line order
}

// SessionHeader is the first line of a session (Format 11).
type SessionHeader struct {
	Schema     string         `json:"$schema,omitempty"`
	Troupe     string         `json:"troupe"`
	Name       string         `json:"name,omitempty"`
	Bundle     string         `json:"bundle,omitempty"`
	Limits     Limits         `json:"limits,omitzero"`
	Scenario   string         `json:"scenario"`
	Characters []Character    `json:"characters,omitempty"`
	Players    []string       `json:"players,omitempty"`
	Worlds     []World        `json:"worlds,omitempty"`
	Head       string         `json:"head,omitempty"`
	Ext        jsontext.Value `json:"ext,omitzero"`
}

// Limits are the players' lines and veils (Format 11).
type Limits struct {
	Lines []string       `json:"lines,omitempty"`
	Veils []string       `json:"veils,omitempty"`
	Ext   jsontext.Value `json:"ext,omitzero"`
}

// Message is a line of a session after its header (Format 11).
type Message struct {
	ID        string         `json:"id"`
	Parent    string         `json:"parent,omitempty"`
	Opening   string         `json:"opening,omitempty"`
	From      string         `json:"from"` // "player", "model" or "system"
	Char      string         `json:"char,omitempty"`
	Content   []Part         `json:"content"`
	At        string         `json:"at,omitempty"` // RFC 3339, with its offset as written
	Changes   []Change       `json:"changes,omitempty"`
	Activated []string       `json:"activated,omitempty"`
	Entries   []Entry        `json:"entries,omitempty"`
	Hidden    bool           `json:"hidden,omitzero"`
	Ext       jsontext.Value `json:"ext,omitzero"`
}

// Part is a part of a message or an opening's content (Format 11). Src and
// Alt are set only for an image part, and Text only for a text part.
type Part struct {
	Type    string         `json:"type,omitempty"` // "" or "text" for text, "image" for an image
	Text    string         `json:"text"`
	Src     string         `json:"src,omitempty"`
	Alt     string         `json:"alt,omitempty"`
	Speaker string         `json:"speaker,omitempty"` // "" for narration
	KnownBy []string       `json:"knownBy,omitempty"` // nil for the witness rule (Format 11.5)
	Ext     jsontext.Value `json:"ext,omitzero"`
}

// Change is a set of state changes a message records (Format 11).
type Change struct {
	By  string                    `json:"by"` // "opening", "player" or "model"
	Set map[string]jsontext.Value `json:"set"`
	Ext jsontext.Value            `json:"ext,omitzero"`
}

// MarshalJSON writes b as a bundle document, its kind included.
func (b Bundle) MarshalJSON() ([]byte, error) {
	type fields Bundle
	return json.Marshal(struct {
		Kind string `json:"kind"`
		fields
	}{"bundle", fields(b)})
}

// MarshalJSON writes h as a session's header line, its kind included.
func (h SessionHeader) MarshalJSON() ([]byte, error) {
	type fields SessionHeader
	return json.Marshal(struct {
		Kind string `json:"kind"`
		fields
	}{"session", fields(h)})
}

// MarshalJSON writes p with the fields its type has: text for a text part,
// and src and alt for an image part.
func (p Part) MarshalJSON() ([]byte, error) {
	if p.Type != "image" {
		return json.Marshal(partFields(p))
	}
	return json.Marshal(struct {
		partFields
		Text string `json:"text,omitzero"`
	}{partFields: partFields(p)})
}
