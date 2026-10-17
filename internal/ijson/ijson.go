// Package ijson reads I-JSON (RFC 7493) as Format 2.2 requires, into the
// generic tree the schema validator takes.
package ijson

import (
	"bytes"
	"encoding/json"
	"encoding/json/jsontext"
	"errors"
	"io"
	"math"
	"strconv"
)

// MaxDepth is the deepest nesting of objects and arrays allowed (Format 14).
const MaxDepth = 64

// Error reports input that isn't one I-JSON value, at the value being read.
type Error struct {
	Pointer jsontext.Pointer
}

func (e *Error) Error() string {
	return "ijson: not I-JSON at " + strconv.Quote(string(e.Pointer))
}

// Decode reads data as exactly one I-JSON value nested at most MaxDepth deep.
// It returns the value as map[string]any, []any, string, json.Number, bool or
// nil, and any other input as an *Error.
//
// jsontext already rejects invalid UTF-8, unpaired surrogates, duplicate names
// and malformed numbers. Decode adds noncharacters, numbers beyond a double's
// range, the depth limit and anything after the value.
func Decode(data []byte) (any, error) {
	dec := jsontext.NewDecoder(bytes.NewReader(data))
	v, err := value(dec)
	if err != nil {
		return nil, err
	}
	if _, err := dec.ReadToken(); !errors.Is(err, io.EOF) {
		return nil, fail(dec, err)
	}
	return v, nil
}

func value(dec *jsontext.Decoder) (any, error) {
	tok, err := dec.ReadToken()
	if err != nil {
		return nil, fail(dec, err)
	}
	switch tok.Kind() {
	case 'n':
		return nil, nil
	case 't', 'f':
		return tok.Bool(), nil
	case '"':
		s := tok.String()
		if hasNoncharacter(s) {
			return nil, fail(dec, nil)
		}
		return s, nil
	case '0':
		text := tok.String()
		if f, _ := strconv.ParseFloat(text, 64); math.IsInf(f, 0) {
			return nil, fail(dec, nil)
		}
		return json.Number(text), nil
	case '{':
		if dec.StackDepth() > MaxDepth {
			return nil, fail(dec, nil)
		}
		obj := map[string]any{}
		for dec.PeekKind() != '}' {
			tok, err := dec.ReadToken()
			if err != nil {
				return nil, fail(dec, err)
			}
			name := tok.String()
			if hasNoncharacter(name) {
				return nil, fail(dec, nil)
			}
			v, err := value(dec)
			if err != nil {
				return nil, err
			}
			obj[name] = v
		}
		if _, err := dec.ReadToken(); err != nil {
			return nil, fail(dec, err)
		}
		return obj, nil
	default: // '['
		if dec.StackDepth() > MaxDepth {
			return nil, fail(dec, nil)
		}
		arr := []any{}
		for dec.PeekKind() != ']' {
			v, err := value(dec)
			if err != nil {
				return nil, err
			}
			arr = append(arr, v)
		}
		if _, err := dec.ReadToken(); err != nil {
			return nil, fail(dec, err)
		}
		return arr, nil
	}
}

// fail returns an Error at the decoder's position, or at the position a
// syntax error gives, which is more precise.
func fail(dec *jsontext.Decoder, err error) error {
	if se, ok := errors.AsType[*jsontext.SyntacticError](err); ok {
		return &Error{Pointer: se.JSONPointer}
	}
	return &Error{Pointer: dec.StackPointer()}
}

// hasNoncharacter reports whether s holds U+FDD0 to U+FDEF, or the last two
// code points of any plane.
func hasNoncharacter(s string) bool {
	for _, r := range s {
		if r >= 0xFDD0 && r <= 0xFDEF || r&0xFFFE == 0xFFFE {
			return true
		}
	}
	return false
}
