package troupe

import "slices"

// narrator stands for the narrator among the characters who know something
// (Format 7.4).
const narrator = "narrator"

// KnownBy returns who knows each part of message m (Format 7.4, 11.5): the IDs
// of the characters who know it, and "narrator" when the narrator does, in
// code point order. before is the state before m's changes: its parent's, or
// the initial state for a root.
//
// A part is known by its speaker, by the characters present after m's changes
// and, unless m is a root, by those present before them: the witness rule. A
// part's knownBy replaces the rule with exactly the listed characters. The
// narrator knows a part unless its knownBy leaves the narrator out, and knows
// every part in narrator mode. Nobody knows a hidden message's parts (Format
// 11.4).
func (st *Story) KnownBy(m *Message, before State) [][]string {
	out := make([][]string, len(m.Content))
	if m.Hidden {
		for i := range out {
			out[i] = []string{}
		}
		return out
	}
	after := st.Apply(before, m)
	for i, part := range m.Content {
		var ids []string
		if part.KnownBy != nil {
			ids = slices.Clone(part.KnownBy)
		} else {
			ids = append(ids, after.s.present...)
			if m.Parent != "" {
				ids = append(ids, before.s.present...)
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
