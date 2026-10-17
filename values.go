package troupe

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"math"
	"slices"
)

// The value limits of Format 14.
const (
	maxStringValue = 4 << 10
	maxSetMembers  = 256
)

// validValue reports whether val is a valid value of v (Format 8.5): of its
// type, using only the members it lists, an integer within ±(2^53 − 1) for an
// int, and within the value limits (Format 14). A variable of a type the spec
// doesn't define fails only the schema.
func validValue(v *Variable, val jsontext.Value) bool {
	var x any
	if json.Unmarshal(val, &x) != nil {
		return false
	}
	switch v.Type {
	case "bool":
		_, ok := x.(bool)
		return ok
	case "int":
		f, ok := x.(float64)
		return ok && f == math.Trunc(f) && math.Abs(f) <= maxSafeInteger
	case "string":
		s, ok := x.(string)
		return ok && len(s) <= maxStringValue
	case "enum":
		s, ok := x.(string)
		return ok && len(s) <= maxStringValue && slices.Contains(v.Values, s)
	case "set":
		members, ok := x.([]any)
		if !ok {
			return false
		}
		distinct := map[string]bool{}
		for _, m := range members {
			s, ok := m.(string)
			if !ok || v.Values != nil && !slices.Contains(v.Values, s) {
				return false
			}
			distinct[s] = true
		}
		return len(distinct) <= maxSetMembers
	}
	return true
}
