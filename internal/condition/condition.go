// Package condition compiles and evaluates conditions: CEL expressions,
// narrowed by Format 10.2.
package condition

import (
	"errors"
	"fmt"
	"slices"

	"cel.dev/cel-go/cel"
	"cel.dev/cel-go/common/ast"
	"cel.dev/cel-go/common/operators"
	"cel.dev/cel-go/common/types"
	"cel.dev/cel-go/common/types/ref"
	"cel.dev/cel-go/interpreter"

	"github.com/TroupesAI/troupe-go/internal/keys"
)

// Kind is the kind of a variable's values.
type Kind int

// The kinds of value (Format 10.1). Dyn is for a variable of a type the spec
// doesn't define, which fails only the schema.
const (
	Bool Kind = iota
	Int
	String
	Strings
	Dyn
)

// Var is a variable an expression may read, by its full name, such as
// "state.bell", "state.kai.nerve" or "own.nerve". A keyed variable is a map
// of its kind's values (Format 8.3).
type Var struct {
	Name  string
	Kind  Kind
	Keyed bool
}

// The names an expression can't write, which hit() and has() expand to.
const (
	hits     = "@hits"
	hasValue = "@has"
)

// builtins are the names of Format 10.1 other than variables.
var builtins = []Var{
	{Name: "scene.location", Kind: String},
	{Name: "scene.present", Kind: Strings},
	{Name: "scene.within", Kind: Strings},
	{Name: "turn", Kind: Int},
	{Name: "elapsed", Kind: Int},
	{Name: "hour", Kind: Int},
	{Name: "weekday", Kind: Int},
	{Name: "opening", Kind: String},
	{Name: "scenario", Kind: String},
	{Name: "char", Kind: String},
	{Name: "user", Kind: String},
	{Name: "self", Kind: String},
	{Name: "players", Kind: Strings},
}

// Env is the environment expressions compile in.
type Env struct {
	env   *cel.Env
	keyed map[string]bool // the keyed variables' names
}

// NewEnv returns the environment of builtins and vars. A name declared twice,
// which a duplicate ID makes, is declared once.
func NewEnv(vars []Var) (*Env, error) {
	e := &Env{keyed: map[string]bool{}}
	opts := []cel.EnvOption{
		// The comprehensions and has() of CEL's standard library aren't
		// allowed; has() is replaced, and hit() added.
		cel.ClearMacros(),
		cel.Macros(
			cel.GlobalMacro("has", 1, expandHas),
			cel.GlobalMacro("hit", 1, expandHit),
		),
		cel.Variable(hits, cel.MapType(cel.StringType, cel.BoolType)),
		cel.Function(hasValue, cel.Overload("has_value", []*cel.Type{cel.DynType}, cel.BoolType,
			cel.OverloadIsNonStrict(),
			cel.UnaryBinding(func(v ref.Val) ref.Val { return types.Bool(!types.IsError(v) && !types.IsUnknown(v)) }))),
		cel.ASTValidators(cel.ValidateHomogeneousAggregateLiterals(), rules{e.keyed}),
	}
	opts = append(opts, e.declare(builtins)...)
	opts = append(opts, e.declare(vars)...)
	env, err := cel.NewEnv(opts...)
	if err != nil {
		return nil, err
	}
	e.env = env
	return e, nil
}

// With returns e with vars added, such as an owner's own variables.
func (e *Env) With(vars []Var) (*Env, error) {
	x := &Env{keyed: map[string]bool{}}
	for name := range e.keyed {
		x.keyed[name] = true
	}
	env, err := e.env.Extend(x.declare(vars)...)
	if err != nil {
		return nil, err
	}
	x.env = env
	return x, nil
}

func (e *Env) declare(vars []Var) []cel.EnvOption {
	var opts []cel.EnvOption
	seen := map[string]bool{}
	for _, v := range vars {
		if seen[v.Name] {
			continue
		}
		seen[v.Name] = true
		t := []*cel.Type{cel.BoolType, cel.IntType, cel.StringType, cel.ListType(cel.StringType), cel.DynType}[v.Kind]
		if v.Keyed {
			t = cel.MapType(cel.StringType, t)
			e.keyed[v.Name] = true
		}
		opts = append(opts, cel.Variable(v.Name, t))
	}
	return opts
}

// Program is a compiled condition.
type Program struct {
	prg  cel.Program
	keys []string // the keys its hit() calls take
}

// Compile compiles a condition. It fails when the condition is a CEL_ERROR:
// it doesn't parse or type-check, isn't a bool, or breaks a rule of Format
// 10.2.
func (e *Env) Compile(expr string) (*Program, error) {
	a, iss := e.env.Compile(expr)
	if iss.Err() != nil {
		return nil, iss.Err()
	}
	if a.OutputType() != cel.BoolType {
		return nil, errors.New("condition: not a bool")
	}
	prg, err := e.env.Program(a)
	if err != nil {
		return nil, err
	}
	p := &Program{prg: prg}
	ast.PreOrderVisit(a.NativeRep().Expr(), ast.NewExprVisitor(func(x ast.Expr) {
		if x.Kind() == ast.CallKind && x.AsCall().FunctionName() == operators.Index {
			if args := x.AsCall().Args(); args[0].Kind() == ast.IdentKind && args[0].AsIdent() == hits {
				p.keys = append(p.keys, string(args[1].AsLiteral().(types.String)))
			}
		}
	}))
	return p, nil
}

// Keys returns the keys the program's hit() calls take.
func (p *Program) Keys() []string {
	return p.keys
}

// Eval evaluates the program, with the names it reads from vars and each of
// its hit() keys found or not in hits. A runtime error makes the condition
// false (Format 10.2).
func (p *Program) Eval(vars interpreter.Activation, hits map[string]bool) bool {
	out, _, err := p.prg.Eval(withHits{vars, hits})
	return err == nil && out == types.True
}

// withHits adds the results of hit() calls to an activation.
type withHits struct {
	interpreter.Activation
	hits map[string]bool
}

func (w withHits) ResolveName(name string) (any, bool) {
	if name == hits {
		return w.hits, true
	}
	return w.Activation.ResolveName(name)
}

// expandHas expands has(x) to a check that selecting x isn't a runtime
// error, whatever the value, which is never an error itself (Format 10.2).
func expandHas(eh cel.MacroExprFactory, _ ast.Expr, args []ast.Expr) (ast.Expr, *cel.Error) {
	if args[0].Kind() != ast.SelectKind {
		return nil, eh.NewError(args[0].ID(), "has() takes a selection")
	}
	return eh.NewCall(hasValue, args[0]), nil
}

// expandHit expands hit(key) to a look-up of whether the key matches the
// scanned text. The key is a string literal, and a regex key must be valid
// (Format 10.2).
func expandHit(eh cel.MacroExprFactory, _ ast.Expr, args []ast.Expr) (ast.Expr, *cel.Error) {
	key, ok := literalString(args[0])
	if !ok {
		return nil, eh.NewError(args[0].ID(), "hit() takes a string literal")
	}
	if _, err := keys.Compile(key); err != nil {
		return nil, eh.NewError(args[0].ID(), fmt.Sprintf("hit(): %v", err))
	}
	return eh.NewCall(operators.Index, eh.NewIdent(hits), eh.NewLiteral(types.String(key))), nil
}

func literalString(x ast.Expr) (string, bool) {
	if x.Kind() != ast.LiteralKind {
		return "", false
	}
	s, ok := x.AsLiteral().(types.String)
	return string(s), ok
}

// allowed are the functions and operators expressions may use (Format 10.2).
var allowed = []string{
	operators.Add, operators.Subtract, operators.Multiply, operators.Divide, operators.Modulo,
	operators.Negate, operators.LogicalNot, operators.LogicalAnd, operators.LogicalOr,
	operators.Equals, operators.NotEquals, operators.Less, operators.LessEquals,
	operators.Greater, operators.GreaterEquals, operators.Conditional,
	operators.Index, operators.In,
	"size", "contains", "startsWith", "endsWith", hasValue,
}

// rules validates what the type check doesn't (Format 10.2): only the allowed
// functions; only bool, int, string, list and map values, so no double,
// bytes, uint or null literal and no type name used as a value; and no char
// or user selected from a keyed variable, which brackets look up.
type rules struct {
	keyed map[string]bool
}

func (rules) Name() string { return "troupe.rules" }

func (r rules) Validate(_ *cel.Env, _ cel.ValidatorConfig, a *ast.AST, iss *cel.Issues) {
	refs := a.ReferenceMap()
	ast.PreOrderVisit(a.Expr(), ast.NewExprVisitor(func(x ast.Expr) {
		if a.GetType(x.ID()).Kind() == types.TypeKind {
			iss.ReportErrorAtID(x.ID(), "a type name isn't a value")
		}
		switch x.Kind() {
		case ast.CallKind:
			if !slices.Contains(allowed, x.AsCall().FunctionName()) {
				iss.ReportErrorAtID(x.ID(), "%s isn't allowed", x.AsCall().FunctionName())
			}
		case ast.LiteralKind:
			switch x.AsLiteral().(type) {
			case types.Double, types.Uint, types.Bytes, types.Null:
				iss.ReportErrorAtID(x.ID(), "a %s literal isn't allowed", x.AsLiteral().Type())
			}
		case ast.SelectKind:
			s := x.AsSelect()
			if ref, ok := refs[s.Operand().ID()]; ok && r.keyed[ref.Name] && (s.FieldName() == "char" || s.FieldName() == "user") {
				iss.ReportErrorAtID(x.ID(), "%s.%s selects a key named %s; brackets look up a computed key", ref.Name, s.FieldName(), s.FieldName())
			}
		}
	}))
}
