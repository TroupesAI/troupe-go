package condition

import (
	"slices"
	"strings"
	"sync"

	"cel.dev/cel-go/cel"
	"cel.dev/cel-go/common/ast"
	"cel.dev/cel-go/common/operators"
	"cel.dev/cel-go/common/types"
)

// Selection is a name an expression selects from state or own, such as
// state.kai.nerve: its names, from state or own, and where each is written,
// as a byte offset in the expression. Key is the string literal it is
// indexed with, as in state.trust['kai'] or 'kai' in state.trust, if any.
type Selection struct {
	Names []string
	At    []int
	Key   *Literal
}

// Literal is a string literal. At is where its value is written, or -1 when
// the literal isn't written as its value between quotes, so the value can't
// be replaced in place. With is the name it is compared with when that name's
// values are IDs, as char is in char == 'kai' and scene.present in 'kai' in
// scene.present, and "" otherwise.
type Literal struct {
	Value string
	At    int
	With  string
}

// The names whose values are an ID, and those whose values are lists of IDs
// (Format 10.1).
var (
	idNames     = []string{"char", "user", "self", "scenario", "opening", "scene.location"}
	idListNames = []string{"players", "scene.present", "scene.within"}
)

// parseEnv parses expressions with the macros conditions compile with.
var parseEnv = sync.OnceValue(func() *cel.Env {
	env, err := cel.NewEnv(macros...)
	if err != nil {
		panic(err)
	}
	return env
})

// Uses returns what expr selects from state and own, and its other string
// literals, other than hit()'s keys, which are text (Format 7.2, 10.1). It
// fails when expr doesn't parse.
func Uses(expr string) ([]Selection, []Literal, error) {
	a, iss := parseEnv().Parse(expr)
	if iss.Err() != nil {
		return nil, nil, iss.Err()
	}
	u := &uses{expr: expr, info: a.NativeRep().SourceInfo(), with: map[int64]string{}}
	// cel-go counts offsets in code points.
	for i := range expr {
		u.offsets = append(u.offsets, i)
	}
	u.offsets = append(u.offsets, len(expr))
	u.visit(a.NativeRep().Expr())
	return u.selections, u.literals, nil
}

type uses struct {
	expr       string
	offsets    []int // the byte offset of each code point, then of the end
	info       *ast.SourceInfo
	with       map[int64]string // the names literals are compared with, by expression ID
	selections []Selection
	literals   []Literal
}

func (u *uses) visit(x ast.Expr) {
	switch x.Kind() {
	case ast.SelectKind:
		if s, ok := u.selection(x); ok {
			u.selections = append(u.selections, s)
			return
		}
		u.visit(x.AsSelect().Operand())
	case ast.CallKind:
		u.call(x.AsCall())
	case ast.ListKind:
		for _, e := range x.AsList().Elements() {
			u.visit(e)
		}
	case ast.MapKind:
		for _, e := range x.AsMap().Entries() {
			u.visit(e.AsMapEntry().Key())
			u.visit(e.AsMapEntry().Value())
		}
	case ast.LiteralKind:
		if _, ok := x.AsLiteral().(types.String); ok {
			u.literals = append(u.literals, u.literal(x))
		}
	}
}

func (u *uses) call(c ast.CallExpr) {
	args := c.Args()
	switch c.FunctionName() {
	case operators.Index:
		if args[0].Kind() == ast.IdentKind && args[0].AsIdent() == hits {
			return
		}
		if s, ok := u.selection(args[0]); ok && isString(args[1]) {
			k := u.literal(args[1])
			s.Key = &k
			u.selections = append(u.selections, s)
			return
		}
	case operators.In:
		if s, ok := u.selection(args[1]); ok && isString(args[0]) {
			k := u.literal(args[0])
			s.Key = &k
			u.selections = append(u.selections, s)
			return
		}
		u.compare(args[0], args[1], idListNames, nil)
		u.compare(args[1], args[0], nil, idNames)
	case operators.Equals, operators.NotEquals:
		u.compare(args[0], args[1], idNames, idListNames)
		u.compare(args[1], args[0], idNames, idListNames)
	}
	if c.IsMemberFunction() {
		u.visit(c.Target())
	}
	for _, a := range args {
		u.visit(a)
	}
}

// compare records the names literals are compared with, for a compared with
// b or in b, or b in a: a string literal a against b, one of names, and each
// item of a list literal a against b, one of listed.
func (u *uses) compare(a, b ast.Expr, names, listed []string) {
	name := u.name(b)
	switch {
	case name == "":
	case isString(a) && slices.Contains(names, name):
		u.with[a.ID()] = name
	case a.Kind() == ast.ListKind && slices.Contains(listed, name):
		for _, e := range a.AsList().Elements() {
			u.with[e.ID()] = name
		}
	}
}

// name returns the name x is, when its values are IDs or lists of them.
func (u *uses) name(x ast.Expr) string {
	var name string
	switch x.Kind() {
	case ast.IdentKind:
		name = x.AsIdent()
	case ast.SelectKind:
		if op := x.AsSelect().Operand(); op.Kind() == ast.IdentKind && op.AsIdent() == "scene" {
			name = "scene." + x.AsSelect().FieldName()
		}
	}
	if slices.Contains(idNames, name) || slices.Contains(idListNames, name) {
		return name
	}
	return ""
}

// selection returns x as a selection from state or own, when it is one.
func (u *uses) selection(x ast.Expr) (Selection, bool) {
	var s Selection
	for x.Kind() == ast.SelectKind {
		sel := x.AsSelect()
		s.Names = append(s.Names, sel.FieldName())
		s.At = append(s.At, u.field(u.offset(x.ID())))
		x = sel.Operand()
	}
	if len(s.Names) == 0 || x.Kind() != ast.IdentKind || x.AsIdent() != "state" && x.AsIdent() != "own" {
		return Selection{}, false
	}
	s.Names = append(s.Names, x.AsIdent())
	s.At = append(s.At, u.offset(x.ID()))
	for i, j := 0, len(s.Names)-1; i < j; i, j = i+1, j-1 {
		s.Names[i], s.Names[j] = s.Names[j], s.Names[i]
		s.At[i], s.At[j] = s.At[j], s.At[i]
	}
	return s, true
}

// field returns where the name a selection's dot, at i, selects is written:
// after the space and comments that may follow the dot.
func (u *uses) field(i int) int {
	for i++; ; {
		switch rest := u.expr[i:]; {
		case strings.HasPrefix(rest, "//"):
			i += strings.IndexByte(rest, '\n') + 1
		case strings.IndexByte(" \t\n\r\f", rest[0]) >= 0:
			i++
		default:
			return i
		}
	}
}

func (u *uses) literal(x ast.Expr) Literal {
	l := Literal{Value: string(x.AsLiteral().(types.String)), At: -1, With: u.with[x.ID()]}
	i := u.offset(x.ID())
	if q := u.expr[i]; (q == '\'' || q == '"') && strings.HasPrefix(u.expr[i+1:], l.Value+string(q)) {
		l.At = i + 1
	}
	return l
}

// offset returns where the expression with the given ID starts, in bytes.
func (u *uses) offset(id int64) int {
	r, _ := u.info.GetOffsetRange(id)
	return u.offsets[r.Start]
}

func isString(x ast.Expr) bool {
	if x.Kind() != ast.LiteralKind {
		return false
	}
	_, ok := x.AsLiteral().(types.String)
	return ok
}
