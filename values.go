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

// validValue reports whether raw is a valid value of v (Format 8.5).
func validValue(v *Variable, raw jsontext.Value) bool {
	_, ok := value(v, raw)
	return ok
}

// value returns raw as a value of v, normalized (Format 8.6): a bool, an
// int64, a string or a []string. It reports false when raw isn't valid: not
// of v's type, using a member v doesn't list, an int outside ±(2^53 − 1), or
// past a value limit (Format 8.5, 14). A variable of a type the spec doesn't
// define fails only the schema, so any value is valid for it, and holds as
// written.
func value(v *Variable, raw jsontext.Value) (any, bool) {
	var x any
	if json.Unmarshal(raw, &x) != nil {
		return nil, false
	}
	switch v.Type {
	case "bool":
		b, ok := x.(bool)
		return b, ok
	case "int":
		f, ok := x.(float64)
		if !ok || f != math.Trunc(f) || math.Abs(f) > maxSafeInteger {
			return nil, false
		}
		return clamp(v, int64(f)), true
	case "string":
		s, ok := x.(string)
		return s, ok && len(s) <= maxStringValue
	case "enum":
		s, ok := x.(string)
		return s, ok && len(s) <= maxStringValue && slices.Contains(v.Values, s)
	case "set":
		items, ok := x.([]any)
		if !ok {
			return nil, false
		}
		var members []string
		for _, item := range items {
			s, ok := item.(string)
			if !ok || len(s) > maxStringValue || v.Values != nil && !slices.Contains(v.Values, s) {
				return nil, false
			}
			if !slices.Contains(members, s) {
				members = append(members, s)
			}
		}
		if len(members) > maxSetMembers {
			return nil, false
		}
		if v.Values != nil {
			// A set with values is ordered as they are declared.
			members = slices.DeleteFunc(slices.Clone(v.Values), func(s string) bool { return !slices.Contains(members, s) })
		}
		if members == nil {
			members = []string{}
		}
		return members, true
	}
	return x, true
}

// zero returns v's default value (Format 8.1).
func zero(v *Variable) any {
	switch v.Type {
	case "bool":
		return false
	case "int":
		return clamp(v, 0)
	case "enum":
		if len(v.Values) > 0 {
			return v.Values[0]
		}
		return ""
	case "set":
		return []string{}
	}
	return ""
}

// clamp raises n to v's min, then lowers it to its max (Format 8.6).
func clamp(v *Variable, n int64) int64 {
	if v.Min != nil {
		n = max(n, *v.Min)
	}
	if v.Max != nil {
		n = min(n, *v.Max)
	}
	return n
}

// isZero reports whether x, a normalized value of v, is v's default.
func isZero(v *Variable, x any) bool {
	if s, ok := x.([]string); ok {
		return len(s) == 0
	}
	return x == zero(v)
}
