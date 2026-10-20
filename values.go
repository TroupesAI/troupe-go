package troupe

import (
	"cmp"
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

// index returns the position of each string of list, the first of any
// repeated, or nil for a nil list. Format 14 bounds a variable's values and
// keys only by the file's size, so they are looked up in an index, never
// searched.
func index(list []string) map[string]int {
	if list == nil {
		return nil
	}
	m := make(map[string]int, len(list))
	for i, s := range list {
		if _, ok := m[s]; !ok {
			m[s] = i
		}
	}
	return m
}

// validValue reports whether raw is a valid value of v (Format 8.5). values
// is index(v.Values).
func validValue(v *Variable, values map[string]int, raw jsontext.Value) bool {
	_, ok := value(v, values, raw)
	return ok
}

// value returns raw as a value of v, normalized (Format 8.6): a bool, an
// int64, a string or a []string. It reports false when raw isn't valid: not
// of v's type, using a member v doesn't list, an int outside ±(2^53 − 1), or
// past a value limit (Format 8.5, 14). values is index(v.Values). A variable
// of a type the spec doesn't define fails only the schema, so any value is
// valid for it, and holds as written.
func value(v *Variable, values map[string]int, raw jsontext.Value) (any, bool) {
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
		_, listed := values[s]
		return s, ok && len(s) <= maxStringValue && listed
	case "set":
		items, ok := x.([]any)
		if !ok {
			return nil, false
		}
		members := []string{}
		seen := map[string]bool{}
		for _, item := range items {
			s, ok := item.(string)
			_, listed := values[s]
			if !ok || len(s) > maxStringValue || values != nil && !listed {
				return nil, false
			}
			if seen[s] {
				continue
			}
			// Stopping at the limit bounds the work, whatever the value's length.
			if len(members) == maxSetMembers {
				return nil, false
			}
			seen[s] = true
			members = append(members, s)
		}
		if values != nil {
			// A set with values is ordered as they are declared.
			slices.SortFunc(members, func(a, b string) int { return cmp.Compare(values[a], values[b]) })
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
