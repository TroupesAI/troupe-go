package troupe

import "encoding/json/jsontext"

// Bundle is a bundle (Format 3).
type Bundle struct {
	Schema     string         `json:"$schema"`
	Troupe     string         `json:"troupe"`
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Tagline    string         `json:"tagline"`
	Version    string         `json:"version"`
	Authors    []string       `json:"authors"`
	Credits    string         `json:"credits"`
	Language   string         `json:"language"`
	Tags       []string       `json:"tags"`
	Rating     string         `json:"rating"` // "general", "mature" or "explicit"
	Warnings   []string       `json:"warnings"`
	Cover      string         `json:"cover"`
	Characters []Character    `json:"characters"`
	Worlds     []World        `json:"worlds"`
	Scenarios  []Scenario     `json:"scenarios"`
	Ext        jsontext.Value `json:"ext"`
}

// Character is a character (Format 4).
type Character struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Aliases     []string       `json:"aliases"`
	Description string         `json:"description"`
	Summary     string         `json:"summary"`
	Examples    [][]Line       `json:"examples"`
	Entries     []Entry        `json:"entries"`
	Variables   []Variable     `json:"variables"`
	Avatar      string         `json:"avatar"`
	Ext         jsontext.Value `json:"ext"`
}

// Line is a line of a character's example exchange (Format 4).
type Line struct {
	Speaker string         `json:"speaker"` // "char", "user" or a character ID; "" for narration
	Text    string         `json:"text"`
	Ext     jsontext.Value `json:"ext"`
}

// World is a world (Format 5).
type World struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Locations   []Location     `json:"locations"`
	Entries     []Entry        `json:"entries"`
	Variables   []Variable     `json:"variables"`
	Ext         jsontext.Value `json:"ext"`
}

// Location is a place in a world (Format 5).
type Location struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parent      string         `json:"parent"`
	Image       string         `json:"image"`
	Ext         jsontext.Value `json:"ext"`
}

// Scenario is a scenario (Format 6).
type Scenario struct {
	ID       string       `json:"id"`
	Name     string       `json:"name"`
	Tagline  string       `json:"tagline"`
	Premise  string       `json:"premise"`
	Briefing string       `json:"briefing"`
	Mode     string       `json:"mode"`     // "characters" or "narrator"; "" for characters
	Narrator string       `json:"narrator"` // "" for "the narrator"
	Cast     []CastMember `json:"cast"`
	// Worlds is nil when absent, which means every bundle world, and points to
	// an empty list for none.
	Worlds    *[]string      `json:"worlds"`
	Openings  []Opening      `json:"openings"`
	Entries   []Entry        `json:"entries"`
	Variables []Variable     `json:"variables"`
	Ext       jsontext.Value `json:"ext"`
}

// CastMember is a member of a scenario's cast (Format 6.1).
type CastMember struct {
	Character string         `json:"character"` // "" for a seat
	Player    bool           `json:"player"`
	Notes     string         `json:"notes"`
	Spoiler   bool           `json:"spoiler"`
	Ext       jsontext.Value `json:"ext"`
}

// Opening is a way a scenario starts (Format 6.3).
type Opening struct {
	ID      string                    `json:"id"`
	Title   string                    `json:"title"`
	Set     map[string]jsontext.Value `json:"set"`
	Content []Part                    `json:"content"`
	Ext     jsontext.Value            `json:"ext"`
}

// Entry is an entry (Format 7).
type Entry struct {
	ID        string         `json:"id"`
	Text      string         `json:"text"`
	Title     string         `json:"title"`
	Keys      []string       `json:"keys"`
	Pull      bool           `json:"pull"`
	When      string         `json:"when"`
	ScanDepth *int           `json:"scanDepth"` // nil for 4
	Once      bool           `json:"once"`
	Sticky    int64          `json:"sticky"`   // 0 for none
	Cooldown  int64          `json:"cooldown"` // 0 for none
	Chance    *int           `json:"chance"`   // a percent; nil for 100
	Group     string         `json:"group"`
	Weight    int            `json:"weight"`   // 0 for 1
	Priority  *int           `json:"priority"` // nil for 50
	Position  string         `json:"position"` // "system" or "context"; "" for context, unless Depth is set
	Depth     *int64         `json:"depth"`
	KnownBy   []string       `json:"knownBy"` // nil for everyone
	Disabled  bool           `json:"disabled"`
	Ext       jsontext.Value `json:"ext"`
}

// Variable is a variable's declaration (Format 8).
type Variable struct {
	ID          string            `json:"id"`
	Type        string            `json:"type"` // "bool", "int", "string", "enum" or "set"
	Values      []string          `json:"values"`
	Labels      map[string]string `json:"labels"`
	Min         *int64            `json:"min"`
	Max         *int64            `json:"max"`
	Initial     jsontext.Value    `json:"initial"` // nil for the type's default
	Of          Keys              `json:"of"`
	Description string            `json:"description"`
	By          []string          `json:"by"` // "player" and "model"
	MaxStep     int64             `json:"maxStep"`
	OneWay      bool              `json:"oneWay"`
	Spoiler     bool              `json:"spoiler"`
	Ext         jsontext.Value    `json:"ext"`
}

// Keys is what a keyed variable has one value for (Format 8.3). Its zero
// value means the variable isn't keyed.
type Keys struct {
	Cast bool     // "cast"
	Any  bool     // "*"
	List []string // the keys listed
}

// Session is a session: its header line, then its messages (Format 11).
type Session struct {
	Header   SessionHeader
	Messages []Message // in line order
}

// SessionHeader is the first line of a session (Format 11).
type SessionHeader struct {
	Schema     string         `json:"$schema"`
	Troupe     string         `json:"troupe"`
	Name       string         `json:"name"`
	Bundle     string         `json:"bundle"`
	Limits     Limits         `json:"limits"`
	Scenario   string         `json:"scenario"`
	Characters []Character    `json:"characters"`
	Players    []string       `json:"players"`
	Worlds     []World        `json:"worlds"`
	Head       string         `json:"head"`
	Ext        jsontext.Value `json:"ext"`
}

// Limits are the players' lines and veils (Format 11).
type Limits struct {
	Lines []string       `json:"lines"`
	Veils []string       `json:"veils"`
	Ext   jsontext.Value `json:"ext"`
}

// Message is a line of a session after its header (Format 11).
type Message struct {
	ID        string         `json:"id"`
	Parent    string         `json:"parent"`
	Opening   string         `json:"opening"`
	From      string         `json:"from"` // "player", "model" or "system"
	Char      string         `json:"char"`
	Content   []Part         `json:"content"`
	At        string         `json:"at"` // RFC 3339, with its offset as written
	Changes   []Change       `json:"changes"`
	Activated []string       `json:"activated"`
	Entries   []Entry        `json:"entries"`
	Hidden    bool           `json:"hidden"`
	Ext       jsontext.Value `json:"ext"`
}

// Part is a part of a message or an opening's content (Format 11). Src and
// Alt are set only for an image part, and Text only for a text part.
type Part struct {
	Type    string         `json:"type"` // "" or "text" for text, "image" for an image
	Text    string         `json:"text"`
	Src     string         `json:"src"`
	Alt     string         `json:"alt"`
	Speaker string         `json:"speaker"` // "" for narration
	KnownBy []string       `json:"knownBy"` // nil for the witness rule (Format 11.5)
	Ext     jsontext.Value `json:"ext"`
}

// Change is a set of state changes a message records (Format 11).
type Change struct {
	By  string                    `json:"by"` // "opening", "player" or "model"
	Set map[string]jsontext.Value `json:"set"`
	Ext jsontext.Value            `json:"ext"`
}
