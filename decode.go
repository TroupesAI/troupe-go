package troupe

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"iter"
	"math"
	"reflect"
	"strconv"
)

// lenient decodes into the typed model whatever passed PARSE. Unknown members
// are ignored, since the schema pass reports them.
var lenient json.Options

func init() {
	// Set here, since decodeValue refers to it.
	lenient = json.WithUnmarshalers(json.UnmarshalFromFunc(decodeValue))
}

// decode decodes a document's bytes into v, a pointer to the typed model.
func decode(data []byte, v any) {
	if err := json.Unmarshal(data, v, lenient); err != nil {
		panic("troupe: decoding the typed model: " + err.Error())
	}
}

// decodeValue decodes the next value into v, leaving v unset when the value
// has the wrong type, so it reads as absent. It returns errors.ErrUnsupported
// for the default decoding where that is lenient already.
func decodeValue(dec *jsontext.Decoder, v any) error {
	if dec.PeekKind() == 'n' {
		return dec.SkipValue() // null is never a value of a field (Format 2.2)
	}
	switch v := v.(type) {
	case *jsontext.Value:
		return errors.ErrUnsupported
	case *string:
		return expect(dec, '"')
	case *bool:
		if k := dec.PeekKind(); k != 't' && k != 'f' {
			return dec.SkipValue()
		}
		return errors.ErrUnsupported
	case *int:
		n, err := readInt(dec)
		if i, ok := toInt(n); ok {
			*v = i
		}
		return err
	case *int64:
		n, err := readInt(dec)
		if n != nil {
			*v = *n
		}
		return err
	case **int:
		n, err := readInt(dec)
		if i, ok := toInt(n); ok {
			*v = &i
		}
		return err
	case **int64:
		n, err := readInt(dec)
		*v = n
		return err
	case **[]string:
		var list []string
		ok, err := decodeList(dec, reflect.ValueOf(&list).Elem())
		if ok {
			*v = &list
		}
		return err
	case *Keys:
		return decodeKeys(dec, v)
	case *Part:
		return decodePart(dec, v)
	}
	rv := reflect.ValueOf(v).Elem()
	switch rv.Kind() {
	case reflect.Struct:
		return expect(dec, '{')
	case reflect.Slice:
		_, err := decodeList(dec, rv)
		return err
	case reflect.Map:
		return decodeMap(dec, rv)
	}
	return errors.ErrUnsupported
}

// expect skips the next value unless it has kind k.
func expect(dec *jsontext.Decoder, k jsontext.Kind) error {
	if dec.PeekKind() != k {
		return dec.SkipValue()
	}
	return errors.ErrUnsupported
}

// readInt reads the next value as an integer, which may be written with a
// zero fraction or an exponent (Format 2.2), and returns nil for any other
// value or one too large for an int64.
func readInt(dec *jsontext.Decoder) (*int64, error) {
	val, err := dec.ReadValue()
	if err != nil || val.Kind() != '0' {
		return nil, err
	}
	if n, err := strconv.ParseInt(string(val), 10, 64); err == nil {
		return &n, nil
	}
	f, _ := strconv.ParseFloat(string(val), 64)
	if f != math.Trunc(f) || math.Abs(f) >= math.MaxInt64 {
		return nil, nil
	}
	n := int64(f)
	return &n, nil
}

// toInt converts n to an int, unless it is nil or too large.
func toInt(n *int64) (int, bool) {
	if n == nil || int64(int(*n)) != *n {
		return 0, false
	}
	return int(*n), true
}

// decodeList decodes the next value into the slice list, only when it is an
// array whose items all have the right type, so every item keeps its index.
func decodeList(dec *jsontext.Decoder, list reflect.Value) (bool, error) {
	val, err := dec.ReadValue()
	if err != nil || val.Kind() != '[' {
		return false, err
	}
	var items []jsontext.Value
	for item := range values(val) {
		if !fits(list.Type().Elem(), item) {
			return false, nil
		}
		items = append(items, item.Clone())
	}
	out := reflect.MakeSlice(list.Type(), len(items), len(items))
	for i, item := range items {
		if err := json.Unmarshal(item, out.Index(i).Addr().Interface(), lenient); err != nil {
			return false, err
		}
	}
	list.Set(out)
	return true, nil
}

// decodeMap decodes the next value into m, leaving out members of the wrong
// type.
func decodeMap(dec *jsontext.Decoder, m reflect.Value) error {
	val, err := dec.ReadValue()
	if err != nil || val.Kind() != '{' {
		return err
	}
	out := reflect.MakeMap(m.Type())
	d := jsontext.NewDecoder(bytes.NewReader(val))
	_, _ = d.ReadToken() // '{'
	for d.PeekKind() != '}' {
		tok, _ := d.ReadToken()
		name := tok.String()
		member, _ := d.ReadValue()
		if !fits(m.Type().Elem(), member) {
			continue
		}
		elem := reflect.New(m.Type().Elem())
		if err := json.Unmarshal(member, elem.Interface(), lenient); err != nil {
			return err
		}
		out.SetMapIndex(reflect.ValueOf(name), elem.Elem())
	}
	m.Set(out)
	return nil
}

var rawType = reflect.TypeFor[jsontext.Value]()

// fits reports whether val has the JSON type that values of t take, for the
// types of list items and map members.
func fits(t reflect.Type, val jsontext.Value) bool {
	switch {
	case t == rawType:
		return val.Kind() != 'n'
	case t.Kind() == reflect.String:
		return val.Kind() == '"'
	case t.Kind() == reflect.Slice:
		return val.Kind() == '['
	default: // a struct
		return val.Kind() == '{'
	}
}

// decodeKeys decodes `of` (Format 8.3): "cast", "*" or a list of keys.
func decodeKeys(dec *jsontext.Decoder, k *Keys) error {
	switch dec.PeekKind() {
	case '"':
		tok, err := dec.ReadToken()
		if err != nil {
			return err
		}
		k.Cast = tok.String() == "cast"
		k.Any = tok.String() == "*"
		return nil
	case '[':
		_, err := decodeList(dec, reflect.ValueOf(&k.List).Elem())
		return err
	}
	return dec.SkipValue()
}

// partFields decodes a Part's fields without decodePart.
type partFields Part

// decodePart decodes a part, keeping only the fields of the interface its
// type selects (Format 13).
func decodePart(dec *jsontext.Decoder, p *Part) error {
	if dec.PeekKind() != '{' {
		return dec.SkipValue()
	}
	if err := json.UnmarshalDecode(dec, (*partFields)(p), lenient); err != nil {
		return err
	}
	if p.Type == "image" {
		p.Text = ""
	} else {
		p.Src, p.Alt = "", ""
	}
	return nil
}

// values yields the items of the array val, each valid until the next.
func values(val jsontext.Value) iter.Seq[jsontext.Value] {
	return func(yield func(jsontext.Value) bool) {
		d := jsontext.NewDecoder(bytes.NewReader(val))
		_, _ = d.ReadToken() // '['
		for d.PeekKind() != ']' {
			item, _ := d.ReadValue()
			if !yield(item) {
				return
			}
		}
	}
}
