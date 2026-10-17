package troupe

import (
	"embed"
	"encoding/json/jsontext"
	"errors"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
)

//go:embed spec/schemas/*.json
var schemaFiles embed.FS

type schemaSet struct {
	bundle, header, message *jsonschema.Schema
}

// schemas compiles the embedded schemas on first use. Each is registered under
// its $id, so references between them never reach the network.
var schemas = sync.OnceValue(func() schemaSet {
	c := jsonschema.NewCompiler()
	c.AssertFormat()
	ids := map[string]string{}
	for _, name := range []string{"bundle", "session-header", "message"} {
		f, err := schemaFiles.Open("spec/schemas/" + name + ".schema.json")
		if err != nil {
			panic(err)
		}
		doc, err := jsonschema.UnmarshalJSON(f)
		if err != nil {
			panic(err)
		}
		ids[name] = doc.(map[string]any)["$id"].(string)
		if err := c.AddResource(ids[name], doc); err != nil {
			panic(err)
		}
	}
	return schemaSet{
		bundle:  c.MustCompile(ids["bundle"]),
		header:  c.MustCompile(ids["session-header"]),
		message: c.MustCompile(ids["message"]),
	}
})

// validate validates doc against sch, reporting each unknown property as
// UNKNOWN_FIELD and each schema failure with unknown properties removed as
// SCHEMA (Format 13, 15.3). It removes the unknown properties from doc.
func validate(sch *jsonschema.Schema, doc any, report func(Code, jsontext.Pointer)) {
	fs := failures(sch, doc)
	unknown := false
	for _, f := range fs {
		props, ok := f.kind.(*kind.AdditionalProperties)
		if !ok || inUnknownPart(f, doc) {
			continue
		}
		obj := lookup(doc, f.loc).(map[string]any)
		for _, p := range props.Properties {
			report(CodeUnknownField, pointer(f.loc).AppendToken(p))
			delete(obj, p)
			unknown = true
		}
	}
	if unknown {
		fs = failures(sch, doc)
	}
	for _, f := range fs {
		report(CodeSchema, pointer(f.loc))
	}
}

// failure is a keyword that fails, at the instance location conformance
// README section 3.4 gives it.
type failure struct {
	kind      jsonschema.ErrorKind
	schemaURL string
	loc       []string
}

// failures validates doc and returns where it fails: at the instance location
// of each keyword that fails, and for an anyOf or oneOf that no branch accepts,
// at the deepest location where a branch fails.
func failures(sch *jsonschema.Schema, doc any) []failure {
	err := sch.Validate(doc)
	if err == nil {
		return nil
	}
	var ve *jsonschema.ValidationError
	if !errors.As(err, &ve) {
		panic(err)
	}
	var out []failure
	var walk func(e *jsonschema.ValidationError, parent []string)
	walk = func(e *jsonschema.ValidationError, parent []string) {
		switch k := e.ErrorKind.(type) {
		case *kind.AnyOf, *kind.OneOf:
			d := deepest(e)
			out = append(out, failure{d.ErrorKind, d.SchemaURL, d.InstanceLocation})
			return
		case *kind.PropertyNames:
			// v6.0.3 keeps this error's InstanceLocation without copying it,
			// so later validation can overwrite its tokens, though not its
			// length. The object whose names fail is the parent's location
			// or, one deeper, the property whose schema has propertyNames.
			loc := slices.Clone(parent)
			if len(e.InstanceLocation) > len(parent) {
				loc = append(loc, path.Base(path.Dir(e.SchemaURL)))
			}
			// A name that fails is at its property, as a key in set is
			// (section 3.4).
			out = append(out, failure{k, e.SchemaURL, append(loc, k.Property)})
			return
		}
		if len(e.Causes) == 0 {
			out = append(out, failure{e.ErrorKind, e.SchemaURL, e.InstanceLocation})
		}
		for _, c := range e.Causes {
			walk(c, e.InstanceLocation)
		}
	}
	walk(ve, nil)
	return out
}

// deepest returns the failure under e with the longest instance location, the
// first of them when several tie.
func deepest(e *jsonschema.ValidationError) *jsonschema.ValidationError {
	best := e
	for _, c := range e.Causes {
		if d := deepest(c); len(d.InstanceLocation) > len(best.InstanceLocation) {
			best = d
		}
	}
	return best
}

// inUnknownPart reports whether e is about a part of an unknown type, whose
// properties are only SCHEMA (Format 13).
func inUnknownPart(f failure, doc any) bool {
	if !strings.Contains(f.schemaURL, "/$defs/part/") {
		return false
	}
	t, ok := lookup(doc, f.loc).(map[string]any)["type"]
	return ok && t != "text" && t != "image"
}

// lookup returns the value at loc in doc.
func lookup(doc any, loc []string) any {
	for _, tok := range loc {
		switch v := doc.(type) {
		case map[string]any:
			doc = v[tok]
		case []any:
			i, _ := strconv.Atoi(tok)
			doc = v[i]
		}
	}
	return doc
}

func pointer(loc []string) jsontext.Pointer {
	var p jsontext.Pointer
	for _, tok := range loc {
		p = p.AppendToken(tok)
	}
	return p
}
