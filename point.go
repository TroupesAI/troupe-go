package troupe

import (
	"errors"
	"maps"
	"math/rand/v2"
	"slices"
	"strings"
	"sync"
	"time"

	"cel.dev/cel-go/interpreter"

	"github.com/TroupesAI/troupe-go/internal/condition"
	"github.com/TroupesAI/troupe-go/internal/keys"
)

// Point is where conditions are evaluated: the parent of the message being
// written, for a character or the narrator (Format 10.1).
type Point struct {
	story *Story
	state *state
	char  string
	// scanned holds the scanned text of each message of the path to the
	// point that isn't hidden and has content, the latest last: the text
	// parts that whoever the message being written is for knows, line
	// breaks written as \n (Format 7.2).
	scanned [][]string
}

// PointAt returns the evaluation point at the last of recent, for a message
// written for char, or for the narrator when char is "". recent are the last
// messages of the path to the point, oldest first, and before is this story's
// state before the first of them: the initial state when it is a root. recent
// needs only as many messages as the deepest scan reaches: at most 100 that
// aren't hidden and have content (Format 7.2). With no messages, the point is
// before any message, at before.
//
// Building a point does the work every condition at it shares, so a caller
// evaluating many conditions builds it once.
func (st *Story) PointAt(before State, recent []Message, char string) *Point {
	p := &Point{story: st, state: before.s.clone(), char: char}
	knower := char
	if knower == "" {
		knower = narrator
	}
	for i := range recent {
		m := &recent[i]
		prior := p.state.clone()
		st.apply(p.state, m)
		if m.Hidden || len(m.Content) == 0 {
			continue
		}
		known := st.knownBy(m, prior, p.state)
		// A root's picks are chosen already (Format 11.1), so any source
		// will do.
		parts := st.renderParts(State{st.scope, p.state}, m, rand.New(rand.NewPCG(0, 0))) //nolint:gosec // G404: see above
		texts := []string{}
		for j, part := range parts {
			if part.Type == "image" || !slices.Contains(known[j], knower) {
				continue
			}
			text := part.Text
			if part.Speaker != "" {
				text = st.scope.characterName(part.Speaker) + ": " + text
			}
			texts = append(texts, keys.Text(text))
		}
		p.scanned = append(p.scanned, texts)
	}
	return p
}

// State returns the state at the point.
func (p *Point) State() State {
	return State{p.story.scope, p.state}
}

// errCondition is Eval's error. It holds no text of the condition, which
// apps log.
var errCondition = errors.New("troupe: the condition is a CEL_ERROR")

// Eval evaluates the condition expr of an entry owned by self, a character,
// world or scenario ID, or "session" for a session entry, which scans
// scanDepth messages: the entry's, 4 when it has none (Format 7.2, 10). It
// returns an error only when expr is a CEL_ERROR; a runtime error makes the
// condition false.
func (p *Point) Eval(expr, self string, scanDepth int) (bool, error) {
	prg, err := p.story.program(expr, self)
	if err != nil {
		return false, errCondition
	}
	hits := map[string]bool{}
	for _, key := range prg.Keys() {
		hits[key] = p.hit(key, scanDepth)
	}
	return prg.Eval(activation{p, self}, hits), nil
}

// hit reports whether key matches the scanned text of the last scanDepth
// messages (Format 7.2).
func (p *Point) hit(key string, scanDepth int) bool {
	k := p.story.key(key)
	if k == nil {
		return false
	}
	for _, texts := range p.scanned[max(0, len(p.scanned)-scanDepth):] {
		if slices.ContainsFunc(texts, k.Match) {
			return true
		}
	}
	return false
}

// program returns expr compiled for an owner, compiling it once per story.
func (st *Story) program(expr, self string) (*condition.Program, error) {
	type compiled struct {
		prg *condition.Program
		err error
	}
	key := self + "\x00" + expr
	if c, ok := st.programs.Load(key); ok {
		return c.(compiled).prg, c.(compiled).err
	}
	env, err := st.env(self)
	var prg *condition.Program
	if err == nil {
		prg, err = env.Compile(expr)
	}
	st.programs.Store(key, compiled{prg, err})
	return prg, err
}

// key returns key compiled, compiling it once per story, or nil when it is
// a regex key that isn't valid.
func (st *Story) key(key string) *keys.Key {
	if k, ok := st.keys.Load(key); ok {
		return k.(*keys.Key)
	}
	k, _ := keys.Compile(key)
	st.keys.Store(key, k)
	return k
}

// env returns the environment of the expressions an owner owns, made once per
// story and owner.
func (st *Story) env(self string) (*condition.Env, error) {
	st.envOnce.Do(func() {
		st.base, st.baseErr = condition.NewEnv(declarations(st.scope.scenario, st.scope.all))
	})
	if st.baseErr != nil {
		return nil, st.baseErr
	}
	if e, ok := st.envs.Load(self); ok {
		return e.(*condition.Env), nil
	}
	e, err := st.base.With(ownDeclarations(st.scope.scenario, st.scope.all, self))
	if err != nil {
		return nil, err
	}
	st.envs.Store(self, e)
	return e, nil
}

// conditions are what a Story keeps to evaluate conditions and match keys.
type conditions struct {
	envOnce  sync.Once
	base     *condition.Env
	baseErr  error
	envs     sync.Map // owner → *condition.Env
	programs sync.Map // owner and expression → compiled program, or its error
	keys     sync.Map // key → *keys.Key, nil when it isn't valid
}

// declarations returns the variables an expression's state type-checks
// against (Format 10.2): the variables of scenario, when it isn't nil, and of
// every character and world.
func declarations(scenario *Scenario, n *names) []condition.Var {
	var vars []condition.Var
	add := func(prefix string, vs []Variable) {
		for _, v := range vs {
			vars = append(vars, conditionVar(prefix+v.ID, v))
		}
	}
	if scenario != nil {
		add("state.", scenario.Variables)
	}
	for _, id := range slices.Sorted(maps.Keys(n.characters)) {
		add("state."+id+".", n.characters[id].Variables)
	}
	for _, id := range slices.Sorted(maps.Keys(n.worlds)) {
		add("state."+id+".", n.worlds[id].Variables)
	}
	return vars
}

// ownDeclarations returns the variables own means for an owner: its own, as
// own.<id>. A session entry has none (Format 8.2).
func ownDeclarations(scenario *Scenario, n *names, self string) []condition.Var {
	vs, _ := n.own(scenario, self)
	var vars []condition.Var
	for _, v := range vs {
		vars = append(vars, conditionVar("own."+v.ID, v))
	}
	return vars
}

// conditionVar returns how an expression reads v under name (Format 10.1).
func conditionVar(name string, v Variable) condition.Var {
	kind := condition.Dyn
	switch v.Type {
	case "bool":
		kind = condition.Bool
	case "int":
		kind = condition.Int
	case "string", "enum":
		kind = condition.String
	case "set":
		kind = condition.Strings
	}
	return condition.Var{Name: name, Kind: kind, Keyed: v.Of.keyed()}
}

// activation resolves the names an expression reads at a point (Format
// 10.1): the state and scene there, and what else depends on the path to it.
// A variable whose owner isn't in use, or an absent key, resolves to nothing,
// which is a runtime error.
type activation struct {
	p    *Point
	self string
}

func (a activation) ResolveName(name string) (any, bool) {
	p, s := a.p, a.p.state
	sc := p.story.scope
	switch name {
	case "scene.location":
		return s.location, true
	case "scene.present":
		return s.present, true
	case "scene.within":
		return sc.within(s.location), true
	case "turn":
		return int64(s.turn), true
	case "elapsed":
		return elapsed(s.parentAt, s.at), true
	case "hour", "weekday":
		t, err := time.Parse(time.RFC3339Nano, s.at)
		switch {
		case err != nil:
			return int64(-1), true
		case name == "hour":
			return int64(t.Hour()), true
		default:
			// ISO: Monday is 1, and Sunday 7.
			return int64((int(t.Weekday())+6)%7 + 1), true
		}
	case "opening":
		return s.opening, true
	case "scenario":
		return sc.scenario.ID, true
	case "char":
		return p.char, true
	case "user":
		return s.user, true
	case "self":
		return a.self, true
	case "players":
		return append([]string{}, sc.players...), true
	}
	if id, ok := strings.CutPrefix(name, "own."); ok {
		name = "state." + id
		if a.self != sc.scenario.ID {
			name = "state." + a.self + "." + id
		}
	}
	x, ok := s.values[name]
	return x, ok
}

func (activation) Parent() interpreter.Activation { return nil }

// elapsed returns the whole seconds from parent to at, rounded down, or 0
// when either is missing or the gap is negative (Format 10.1).
func elapsed(parent, at string) int64 {
	from, err1 := time.Parse(time.RFC3339Nano, parent)
	to, err2 := time.Parse(time.RFC3339Nano, at)
	if err1 != nil || err2 != nil || to.Before(from) {
		return 0
	}
	// From the seconds, since a time.Duration holds only about 292 years.
	s := to.Unix() - from.Unix()
	if to.Nanosecond() < from.Nanosecond() {
		s--
	}
	return s
}

// within returns a location and its ancestors, innermost first; none for no
// location (Format 10.1).
func (sc *scope) within(id string) []string {
	out := []string{}
	for l := sc.all.locations[id]; l != nil && !slices.Contains(out, l.ID); l = sc.all.locations[l.Parent] {
		out = append(out, l.ID)
	}
	return out
}
