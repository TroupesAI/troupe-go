package troupe

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
)

// Code is the code of a check (Format 15.3).
type Code string

// The codes reading reports.
const (
	CodeParse              Code = "PARSE"
	CodeUnsupportedVersion Code = "UNSUPPORTED_VERSION"
	CodeSchema             Code = "SCHEMA"
	CodeUnknownField       Code = "UNKNOWN_FIELD"
)

// Diagnostic is a check a document fails, and where. It never holds text from
// the document other than the names in its location.
type Diagnostic struct {
	Code Code
	// File is the path within a package; "" outside one, or when the
	// diagnostic is about a ZIP entry whose name isn't a package path.
	File string
	// Line is the line of a session, from 1 for its header; 0 in a bundle.
	Line int
	// Message is the message's ID, when its line has a valid one.
	Message string
	// Pointer is a JSON Pointer within the document.
	Pointer string
}

// Location gives where the diagnostic is in the conformance suite's form,
// without the file: "/scenarios/0/when" in a bundle, "header/players" in a
// session's header, and "message:3/changes/0" in a message. A message line
// without a valid ID is "line:<n>" instead.
func (d Diagnostic) Location() string {
	switch {
	case d.Line == 0:
		return d.Pointer
	case d.Line == 1:
		return "header" + d.Pointer
	case d.Message != "":
		return "message:" + d.Message + d.Pointer
	default:
		return "line:" + strconv.Itoa(d.Line) + d.Pointer
	}
}

// sortDiagnostics sorts ds by file, line, pointer and code, comparing array
// indices as numbers, and removes duplicates.
func sortDiagnostics(ds []Diagnostic) []Diagnostic {
	slices.SortFunc(ds, func(a, b Diagnostic) int {
		return cmp.Or(
			strings.Compare(a.File, b.File),
			cmp.Compare(a.Line, b.Line),
			comparePointers(a.Pointer, b.Pointer),
			strings.Compare(string(a.Code), string(b.Code)),
		)
	})
	return slices.Compact(ds)
}

func comparePointers(a, b string) int {
	as, bs := strings.Split(a, "/"), strings.Split(b, "/")
	for i := range min(len(as), len(bs)) {
		if c := compareTokens(as[i], bs[i]); c != 0 {
			return c
		}
	}
	return cmp.Compare(len(as), len(bs))
}

func compareTokens(a, b string) int {
	if m, err := strconv.Atoi(a); err == nil {
		if n, err := strconv.Atoi(b); err == nil {
			return cmp.Compare(m, n)
		}
	}
	return strings.Compare(a, b)
}
