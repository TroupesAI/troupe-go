package troupe

import (
	"encoding/json/jsontext"
	"iter"
	"regexp"
	"strconv"
	"strings"
)

// The limits of Format 14, other than the value limits, which are checks of
// their own (Format 8.5).
const (
	maxBundleSize     = 32 << 20
	maxSessionSize    = 256 << 20
	maxMessages       = 100_000
	maxZipEntries     = 10_000
	maxUnpackedSize   = 512 << 20
	maxEntries        = 10_000
	maxVariables      = 1_000
	maxText           = 1 << 20
	maxSessionEntries = 10_000
	maxCEL            = 4 << 10
	maxCast           = 256
)

// packagePath is the grammar of a package's file paths (Format 12.3).
var packagePath = regexp.MustCompile(`^[A-Za-z0-9_-][A-Za-z0-9._-]*(/[A-Za-z0-9_-][A-Za-z0-9._-]*)*$`)

// isPackageFile reports whether name can be a file of a package: it follows
// the grammar, with segments of at most 255 bytes (Format 12.3).
func isPackageFile(name string) bool {
	if !packagePath.MatchString(name) {
		return false
	}
	for seg := range strings.SplitSeq(name, "/") {
		if len(seg) > 255 {
			return false
		}
	}
	return true
}

// scan checks a document's limits and file references before the schema pass,
// so no other check walks what a limit bounds (Format 14). It reads the
// document leniently: values of the wrong type are the schema's to report.
type scan struct {
	report func(jsontext.Pointer) // reports PARSE
	// files reports whether a package holds a file; nil outside a package,
	// where package paths aren't allowed (Format 12.2).
	files     func(string) bool
	entries   int
	variables int
	cast      int
}

func (s *scan) bundle(doc map[string]any) {
	s.image("", doc, "cover")
	for p, c := range each(doc, "", "characters") {
		s.character(p, c)
	}
	for p, w := range each(doc, "", "worlds") {
		s.world(p, w)
	}
	for p, sc := range each(doc, "", "scenarios") {
		s.owner(p, sc)
		for op, o := range each(sc, p, "openings") {
			s.content(op, o)
		}
	}
}

// header checks a session's header. The cast's other characters come from
// the bundle, so only those the session adds are counted here.
func (s *scan) header(doc map[string]any) {
	for p, c := range each(doc, "", "characters") {
		if s.cast++; s.cast == maxCast+1 {
			s.report(p)
		}
		s.character(p, c)
	}
	for p, w := range each(doc, "", "worlds") {
		s.world(p, w)
	}
}

func (s *scan) message(doc map[string]any) {
	s.content("", doc)
	for p, e := range each(doc, "", "entries") {
		s.entry(p, e)
	}
}

func (s *scan) character(p jsontext.Pointer, c map[string]any) {
	s.text(p, c, "description")
	s.image(p, c, "avatar")
	s.owner(p, c)
}

func (s *scan) world(p jsontext.Pointer, w map[string]any) {
	s.text(p, w, "description")
	for lp, l := range each(w, p, "locations") {
		s.text(lp, l, "description")
		s.image(lp, l, "image")
	}
	s.owner(p, w)
}

// owner checks a character's, world's or scenario's entries and variables.
func (s *scan) owner(p jsontext.Pointer, o map[string]any) {
	for ep, e := range each(o, p, "entries") {
		if s.entries++; s.entries == maxEntries+1 {
			s.report(ep)
		}
		s.entry(ep, e)
	}
	for vp, v := range each(o, p, "variables") {
		if s.variables++; s.variables == maxVariables+1 {
			s.report(vp)
		}
		s.text(vp, v, "description")
	}
}

func (s *scan) entry(p jsontext.Pointer, e map[string]any) {
	s.text(p, e, "text")
	if when, ok := e["when"].(string); ok && len(when) > maxCEL {
		s.report(p.AppendToken("when"))
	}
}

// content checks the parts of a message or an opening.
func (s *scan) content(p jsontext.Pointer, o map[string]any) {
	for pp, part := range each(o, p, "content") {
		s.text(pp, part, "text")
		if part["type"] == "image" {
			s.image(pp, part, "src")
		}
	}
}

func (s *scan) text(p jsontext.Pointer, o map[string]any, key string) {
	if t, ok := o[key].(string); ok && len(t) > maxText {
		s.report(p.AppendToken(key))
	}
}

// image checks a field that holds a file: a package path is allowed only in
// a package, and must name one of its files (Format 12.2, 12.3).
func (s *scan) image(p jsontext.Pointer, o map[string]any, key string) {
	path, ok := o[key].(string)
	if ok && packagePath.MatchString(path) && (s.files == nil || !s.files(path)) {
		s.report(p.AppendToken(key))
	}
}

// each yields the objects in the array o[key], with their pointers.
func each(o map[string]any, p jsontext.Pointer, key string) iter.Seq2[jsontext.Pointer, map[string]any] {
	return func(yield func(jsontext.Pointer, map[string]any) bool) {
		items, _ := o[key].([]any)
		for i, item := range items {
			if obj, ok := item.(map[string]any); ok && !yield(p.AppendToken(key).AppendToken(strconv.Itoa(i)), obj) {
				return
			}
		}
	}
}
