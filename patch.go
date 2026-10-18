package troupe

import (
	"bytes"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/TroupesAI/troupe-go/internal/ijson"
)

// Operation is an operation of a JSON Patch (RFC 6902). It reads and writes as
// RFC 6902 writes it, so a patch sent as JSON decodes into []Operation.
type Operation struct {
	Op    string         `json:"op"`             // "add", "remove", "replace", "move" or "test"
	Path  string         `json:"path"`           // a JSON Pointer
	From  string         `json:"from,omitempty"` // for move
	Value jsontext.Value `json:"value,omitzero"` // for add, replace and test
}

// ErrTestFailed is what PatchBundle's error wraps when a test operation finds
// another value at its path.
var ErrTestFailed = errors.New("the value differs")

// PatchBundle returns doc, a bundle's JSON, with patch applied as RFC 6902
// applies it: every operation, in order, or none. Copy isn't supported.
//
// doc needs to be JSON, but not to conform: only the values along the
// operations' paths are read. Every byte the patch doesn't change is kept, so
// every ext stays as written (Format 13). An operation rewrites only the value
// at its path, with the comma beside it when it adds or removes one; a value
// it adds is written compactly, and spaced like its neighbors.
//
// Values must be I-JSON, nested at most 64 deep where they are added, and the
// bundle at most 32 MiB after every operation (Format 2.2, 14). The result can
// still fail reading, such as with PARSE for another limit of Format 14, so a
// caller reads it for its diagnostics. An operation's work grows at most with
// the bundle's size, so a caller bounds a patch's by how many operations it
// takes. The error names the operation that fails, and wraps ErrTestFailed
// for a test.
func PatchBundle(doc []byte, patch []Operation) ([]byte, error) {
	if len(doc) > maxBundleSize {
		return nil, errors.New("troupe: the document is over 32 MiB")
	}
	d, err := parseDocument(doc)
	if err != nil {
		return nil, err
	}
	for i, op := range patch {
		if err := d.apply(op); err != nil {
			return nil, fmt.Errorf("troupe: operation %d: %w", i, err)
		}
		if d.size() > maxBundleSize {
			return nil, fmt.Errorf("troupe: operation %d: the bundle would be over 32 MiB", i)
		}
	}
	out := make([]byte, 0, d.size())
	out = d.root.append(append(out, d.before...))
	return append(out, d.after...), nil
}

// document is a JSON document being patched: its value, and the space around
// it.
type document struct {
	before, after []byte
	root          *node
}

// parseDocument reads doc as JSON. Its values are read again only as
// operations reach into them, so they are checked once, here.
func parseDocument(doc []byte) (*document, error) {
	dec := jsontext.NewDecoder(bytes.NewReader(doc))
	v, err := dec.ReadValue()
	end := int(dec.InputOffset())
	if err == nil {
		_, err = dec.ReadToken()
	}
	if !errors.Is(err, io.EOF) {
		return nil, errors.New("troupe: the document isn't JSON")
	}
	start := end - len(v)
	return &document{before: doc[:start], after: doc[end:], root: &node{raw: doc[start:end]}}, nil
}

func (d *document) size() int {
	return len(d.before) + d.root.size() + len(d.after)
}

func (d *document) apply(op Operation) error {
	path, err := tokens("path", op.Path)
	if err != nil {
		return err
	}
	switch op.Op {
	case "add":
		v, err := newValue(op.Value, len(path))
		if err != nil {
			return err
		}
		return d.add(path, v)
	case "remove":
		_, err := d.remove(path)
		return err
	case "replace":
		v, err := newValue(op.Value, len(path))
		if err != nil {
			return err
		}
		return d.replace(path, v)
	case "move":
		from, err := tokens("from", op.From)
		if err != nil {
			return err
		}
		return d.move(from, path)
	case "test":
		if op.Value == nil {
			return errors.New("the value is missing")
		}
		x, err := ijson.Decode(op.Value)
		if err != nil {
			return errors.New("the value isn't I-JSON")
		}
		n, err := d.get(path)
		if err != nil {
			return err
		}
		if !n.equal(x) {
			return ErrTestFailed
		}
		return nil
	}
	return errors.New("the op isn't add, remove, replace, move or test")
}

// tokens returns the tokens of p, a JSON Pointer in the field of an
// operation.
func tokens(field, p string) ([]string, error) {
	ptr := jsontext.Pointer(p)
	if !ptr.IsValid() {
		return nil, fmt.Errorf("the %s isn't a JSON Pointer", field)
	}
	return slices.Collect(ptr.Tokens()), nil
}

// newValue returns an operation's value as a node at a path of depth tokens.
func newValue(raw jsontext.Value, depth int) (*node, error) {
	if raw == nil {
		return nil, errors.New("the value is missing")
	}
	if _, err := ijson.Decode(raw); err != nil {
		return nil, errors.New("the value isn't I-JSON")
	}
	v := raw.Clone()
	_ = v.Compact() // it is JSON
	n := &node{raw: v}
	if depth+n.depth() > ijson.MaxDepth {
		return nil, errors.New("the value would be nested more than 64 deep")
	}
	return n, nil
}

// get returns the value at path.
func (d *document) get(path []string) (*node, error) {
	n := d.root
	for _, tok := range path {
		i, ok := n.index(tok)
		if !ok {
			return nil, errors.New("the path names no value")
		}
		n = n.elems[i].value
	}
	return n, nil
}

func (d *document) add(path []string, v *node) error {
	if len(path) == 0 {
		d.root = v
		return nil
	}
	parent, err := d.get(path[:len(path)-1])
	if err != nil {
		return err
	}
	tok := path[len(path)-1]
	switch parent.kind() {
	case '{':
		if i, ok := parent.index(tok); ok {
			parent.elems[i].value = v
		} else {
			parent.insert(len(parent.elems), tok, v)
		}
		return nil
	case '[':
		parent.expand()
		i, ok := len(parent.elems), tok == "-"
		if !ok {
			i, ok = arrayIndex(tok)
		}
		if ok && i <= len(parent.elems) {
			parent.insert(i, "", v)
			return nil
		}
	}
	return errors.New("the path names no place for a value")
}

func (d *document) remove(path []string) (*node, error) {
	if len(path) == 0 {
		return nil, errors.New("the whole document can't be removed")
	}
	parent, err := d.get(path[:len(path)-1])
	if err != nil {
		return nil, err
	}
	i, ok := parent.index(path[len(path)-1])
	if !ok {
		return nil, errors.New("the path names no value")
	}
	return parent.remove(i), nil
}

func (d *document) replace(path []string, v *node) error {
	if len(path) == 0 {
		d.root = v
		return nil
	}
	parent, err := d.get(path[:len(path)-1])
	if err != nil {
		return err
	}
	i, ok := parent.index(path[len(path)-1])
	if !ok {
		return errors.New("the path names no value")
	}
	parent.elems[i].value = v
	return nil
}

// move removes the value at from, then adds it at path (RFC 6902 section
// 4.4).
func (d *document) move(from, path []string) error {
	if _, err := d.get(from); err != nil {
		return errors.New("the from names no value")
	}
	if len(path) > len(from) && slices.Equal(path[:len(from)], from) {
		return errors.New("the path is inside the from")
	}
	if slices.Equal(from, path) {
		return nil
	}
	v, err := d.remove(from)
	if err != nil {
		return err
	}
	if len(path)+v.depth() > ijson.MaxDepth {
		return errors.New("the value would be nested more than 64 deep")
	}
	return d.add(path, v)
}

// arrayIndex reads tok as an array index: 0, or digits that don't start with
// 0 (RFC 6901 section 4).
func arrayIndex(tok string) (int, bool) {
	if tok == "" || tok[0] == '0' && tok != "0" || strings.Trim(tok, "0123456789") != "" {
		return 0, false
	}
	i, err := strconv.Atoi(tok)
	return i, err == nil
}

// node is a JSON value of a document being patched. It keeps the value's bytes
// as written until an operation reaches inside it, and then those of its
// members or items, and of everything between them.
type node struct {
	raw []byte // the value as written; nil once expanded
	// An expanded object or array: its opening brace or bracket, its
	// members or items, and what follows the last one before it closes.
	open  jsontext.Kind
	elems []elem
	end   []byte
}

// elem is a member of an object or an item of an array.
type elem struct {
	// before is what precedes the value: the comma, unless it is the first,
	// then space, and a member's name and colon.
	before []byte
	name   string // a member's name
	value  *node
}

func (n *node) kind() jsontext.Kind {
	if n.raw == nil {
		return n.open
	}
	return jsontext.Value(n.raw).Kind()
}

// expand reads n's members or items, when n is an object or array that isn't
// expanded already.
func (n *node) expand() {
	if k := n.kind(); n.raw == nil || k != '{' && k != '[' {
		return
	}
	// parseDocument checked the JSON, so reading it can't fail.
	dec := jsontext.NewDecoder(bytes.NewReader(n.raw))
	_, _ = dec.ReadToken()
	n.open = n.kind()
	last := int(dec.InputOffset())
	for k := dec.PeekKind(); k != '}' && k != ']'; k = dec.PeekKind() {
		var e elem
		if n.open == '{' {
			name, _ := dec.ReadToken()
			e.name = name.String()
		}
		v, _ := dec.ReadValue()
		end := int(dec.InputOffset())
		e.before = n.raw[last : end-len(v)]
		e.value = &node{raw: n.raw[end-len(v) : end]}
		n.elems = append(n.elems, e)
		last = end
	}
	n.end = n.raw[last : len(n.raw)-1]
	n.raw = nil
}

// index returns the index of the member or item tok names, when n is an
// object or array that has it.
func (n *node) index(tok string) (int, bool) {
	n.expand()
	switch n.kind() {
	case '{':
		i := slices.IndexFunc(n.elems, func(e elem) bool { return e.name == tok })
		return i, i >= 0
	case '[':
		i, ok := arrayIndex(tok)
		return i, ok && i < len(n.elems)
	}
	return 0, false
}

// insert puts a member or item at index i of n, an expanded object or array,
// spaced like the one it displaces, or at the end like the last one.
func (n *node) insert(i int, name string, v *node) {
	e := elem{name: name, value: v}
	var key []byte
	if n.open == '{' {
		key, _ = jsontext.AppendQuote(nil, name) // the pointer was valid UTF-8
	}
	switch {
	case len(n.elems) == 0 && key != nil:
		e.before = append(key, ':')
	case len(n.elems) > 0:
		like := n.elems[min(i, len(n.elems)-1)].before
		space := like
		if j := bytes.IndexAny(space, `,"`); j >= 0 && space[j] == ',' {
			space = space[j+1:]
		}
		if key != nil {
			space = space[:bytes.IndexByte(space, '"')]
			key = append(key, like[bytes.LastIndexByte(like, '"')+1:]...)
		}
		e.before = slices.Concat([]byte{','}, space, key)
		if i == 0 {
			e.before = e.before[1:]
			n.elems[0].before = slices.Concat([]byte{','}, n.elems[0].before)
		}
	}
	n.elems = slices.Insert(n.elems, i, e)
}

// remove removes the member or item at index i of n, an expanded object or
// array, and returns its value.
func (n *node) remove(i int) *node {
	v := n.elems[i].value
	if i == 0 && len(n.elems) > 1 {
		// The next becomes the first, without the comma before it.
		next := n.elems[1].before
		n.elems[1].before = next[bytes.IndexByte(next, ',')+1:]
	}
	n.elems = slices.Delete(n.elems, i, i+1)
	return v
}

func (n *node) size() int {
	if n.raw != nil {
		return len(n.raw)
	}
	size := len("{}") + len(n.end)
	for _, e := range n.elems {
		size += len(e.before) + e.value.size()
	}
	return size
}

// depth returns how many arrays and objects n nests, itself included; 0 for
// any other value.
func (n *node) depth() int {
	if n.raw == nil {
		depth := 0
		for _, e := range n.elems {
			depth = max(depth, e.value.depth())
		}
		return depth + 1
	}
	dec := jsontext.NewDecoder(bytes.NewReader(n.raw))
	depth := 0
	for {
		if _, err := dec.ReadToken(); err != nil {
			return depth
		}
		depth = max(depth, dec.StackDepth())
	}
}

// equal reports whether n's value is x, a value as ijson.Decode returns it, so
// numbers are equal when they mean the same double (RFC 6902 section 4.6,
// Format 2.2).
func (n *node) equal(x any) bool {
	switch x := x.(type) {
	case map[string]any:
		if n.kind() != '{' {
			return false
		}
		n.expand()
		if len(n.elems) != len(x) {
			return false
		}
		for _, e := range n.elems {
			if v, ok := x[e.name]; !ok || !e.value.equal(v) {
				return false
			}
		}
		return true
	case []any:
		if n.kind() != '[' {
			return false
		}
		n.expand()
		if len(n.elems) != len(x) {
			return false
		}
		for i, e := range n.elems {
			if !e.value.equal(x[i]) {
				return false
			}
		}
		return true
	}
	if k := n.kind(); k == '{' || k == '[' {
		return false
	}
	v, err := ijson.Decode(n.raw)
	return err == nil && v == x
}

// append appends n as written, or as its members or items are now.
func (n *node) append(b []byte) []byte {
	if n.raw != nil {
		return append(b, n.raw...)
	}
	b = append(b, byte(n.open))
	for _, e := range n.elems {
		b = e.value.append(append(b, e.before...))
	}
	b = append(b, n.end...)
	if n.open == '{' {
		return append(b, '}')
	}
	return append(b, ']')
}
