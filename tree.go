package troupe

// walkPaths visits a session's messages depth first, calling enter on the way
// down to a message and leave on the way back up, so what enter adds and leave
// removes is always the path to the current message. A message whose parent
// names no message on an earlier line is a root (Format 11.1).
func walkPaths(msgs []Message, enter, leave func(i int)) {
	roots := len(msgs) // the index whose children are the roots
	children := make([][]int, roots+1)
	index := map[string]int{}
	for i, m := range msgs {
		parent, ok := index[m.Parent]
		if !ok {
			parent = roots
		}
		children[parent] = append(children[parent], i)
		if _, dup := index[m.ID]; !dup {
			index[m.ID] = i
		}
	}
	type visit struct {
		message int
		leave   bool
	}
	var stack []visit
	for _, c := range children[roots] {
		stack = append(stack, visit{c, false})
	}
	for len(stack) > 0 {
		v := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if v.leave {
			leave(v.message)
			continue
		}
		enter(v.message)
		stack = append(stack, visit{v.message, true})
		for _, c := range children[v.message] {
			stack = append(stack, visit{c, false})
		}
	}
}
