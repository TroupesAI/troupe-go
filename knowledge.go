package troupe

import "slices"

// narrator stands for the narrator among the characters who know something
// (Format 7.4).
const narrator = "narrator"

// KnownBy returns who knows each part of message m (Format 7.4, 11.5): the IDs
// of the characters who know it, and "narrator" when the narrator does, in
// code point order. before is this story's state before m: its parent's, or
// the initial state for a root.
//
// A part is known by its speaker, by the characters present after m's changes
// and, unless m is a root, by those present before them: the witness rule. A
// part's knownBy replaces the rule with exactly the listed characters. The
// narrator knows a part unless its knownBy leaves the narrator out, and knows
// every part in narrator mode. Nobody knows a hidden message's parts (Format
// 11.4).
func (st *Story) KnownBy(before State, m *Message) [][]string {
	return st.knownBy(m, before.s, st.Apply(before, m).s)
}

// knownBy returns who knows each part of m, given the states before and after
// it.
func (st *Story) knownBy(m *Message, before, after *state) [][]string {
	out := make([][]string, len(m.Content))
	if m.Hidden {
		for i := range out {
			out[i] = []string{}
		}
		return out
	}
	for i, part := range m.Content {
		var ids []string
		if part.KnownBy != nil {
			ids = slices.Clone(part.KnownBy)
		} else {
			ids = append(ids, after.present...)
			if m.Parent != "" {
				ids = append(ids, before.present...)
			}
			if part.Speaker != "" {
				ids = append(ids, part.Speaker)
			}
			ids = append(ids, narrator)
		}
		if st.scope.scenario.Mode == "narrator" {
			ids = append(ids, narrator)
		}
		slices.Sort(ids)
		out[i] = slices.Compact(ids)
	}
	return out
}
