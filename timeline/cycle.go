package timeline

import (
	"fmt"
	"strings"
)

// CycleError 描述复合片段引用图中的一个环。
// Path 是环上的复合片段 ID，首尾相同，例如 [A B C A]。
type CycleError struct {
	Path []string
}

func (e *CycleError) Error() string {
	return fmt.Sprintf("timeline: circular compound reference: %s", strings.Join(e.Path, " -> "))
}

type cycleState int

const (
	cycleUnvisited cycleState = iota
	cycleInStack
	cycleDone
)

// CheckCycles 只做循环引用检测，与普通结构校验分离。
// 复合片段之间通过 KindRef 条目构成有向边；发现环时返回 *CycleError。
func (p *Project) CheckCycles() error {
	state := map[string]cycleState{}
	stack := []string{}

	var visit func(id string) error
	visit = func(id string) error {
		switch state[id] {
		case cycleInStack:
			for i, sid := range stack {
				if sid == id {
					ring := append(append([]string{}, stack[i:]...), id)
					return &CycleError{Path: ring}
				}
			}
			return &CycleError{Path: []string{id, id}}
		case cycleDone:
			return nil
		}
		state[id] = cycleInStack
		stack = append(stack, id)

		c := p.Compounds[id]
		if c != nil {
			for _, it := range c.Items {
				if it != nil && it.Kind == KindRef {
					if err := visit(it.CompoundID); err != nil {
						return err
					}
				}
			}
		}

		stack = stack[:len(stack)-1]
		state[id] = cycleDone
		return nil
	}

	for id := range p.Compounds {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}
