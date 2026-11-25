package eac3

import (
	"fmt"
	"strings"
)

// Mermaid 输出解码前节目状态图；边上的原因码就是每次受控重建的依据。
func (g *Graph) Mermaid() string {
	if g == nil {
		return "flowchart LR\n"
	}
	var b strings.Builder
	b.WriteString("flowchart LR\n")
	index := make(map[string]int, len(g.StateOrder))
	for i, id := range g.StateOrder {
		index[id] = i
		st := g.States[id]
		fmt.Fprintf(&b, "  N%d[\"%s<br/>%s<br/>%d Hz / %d ch / %d samples<br/>DRC %s\"]\n",
			i, escapeMermaid(st.ProgramID), escapeMermaid(string(st.Mode)),
			st.Spec.SampleRateHz, st.Spec.Channel.Channels,
			st.Spec.Boundary.SamplesPerFrame, escapeMermaid(st.Spec.DRC.identity()))
	}
	for _, tr := range g.Transitions {
		from, okFrom := index[tr.FromID]
		to, okTo := index[tr.ToID]
		if !okTo {
			continue
		}
		if !okFrom {
			fmt.Fprintf(&b, "  Start((\"start\")) -->|%s| N%d\n", edgeLabel(tr), to)
			continue
		}
		style := "-->"
		if tr.NoOp {
			style = "-.->|NO_OP|"
			fmt.Fprintf(&b, "  N%d %s N%d\n", from, style, to)
			continue
		}
		fmt.Fprintf(&b, "  N%d -->|%s| N%d\n", from, edgeLabel(tr), to)
	}
	if g.CurrentID != "" {
		if i, ok := index[g.CurrentID]; ok {
			fmt.Fprintf(&b, "  classDef current fill:#d9f2ff,stroke:#0b6fa4,stroke-width:2px;\n")
			fmt.Fprintf(&b, "  class N%d current;\n", i)
		}
	}
	return b.String()
}

func edgeLabel(tr Switch) string {
	codes := make([]string, 0, len(tr.Reasons))
	for _, r := range tr.Reasons {
		if r.Code != ReasonUserSelection {
			codes = append(codes, r.Code)
		}
	}
	if len(codes) == 0 && len(tr.Reasons) > 0 {
		codes = append(codes, tr.Reasons[0].Code)
	}
	label := fmt.Sprintf("#%d %s", tr.Sequence, strings.Join(codes, " + "))
	return escapeMermaid(label)
}

func escapeMermaid(s string) string {
	s = strings.ReplaceAll(s, "\"", "'")
	s = strings.ReplaceAll(s, "|", "/")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

// Markdown 输出每次切换的原因、边界决策和有序事件。
func (g *Graph) Markdown() string {
	var b strings.Builder
	b.WriteString("# E-AC-3 解码前节目状态图报告\n\n")
	fmt.Fprintf(&b, "- 节目拓扑：%d 个\n- 状态节点：%d 个\n- 切换记录：%d 条\n\n",
		len(g.Programs), len(g.StateOrder), len(g.Transitions))
	b.WriteString("```mermaid\n")
	b.WriteString(g.Mermaid())
	b.WriteString("```\n\n")

	for _, tr := range g.Transitions {
		title := fmt.Sprintf("切换 #%d", tr.Sequence)
		if tr.NoOp {
			title += "（无操作）"
		}
		fmt.Fprintf(&b, "## %s\n\n", title)
		fmt.Fprintf(&b, "- 路径：`%s` → `%s`\n", orEmpty(tr.FromID, "∅"), tr.ToID)
		fmt.Fprintf(&b, "- 请求：program=`%s` mode=`%s`\n", tr.Request.ProgramID, tr.Request.Mode)
		if len(tr.Reasons) > 0 {
			b.WriteString("- 原因：\n")
			for _, r := range tr.Reasons {
				if r.Old != "" || r.New != "" {
					fmt.Fprintf(&b, "  - `%s` %s（`%s` → `%s`）\n", r.Code, r.Message, r.Old, r.New)
				} else {
					fmt.Fprintf(&b, "  - `%s` %s\n", r.Code, r.Message)
				}
			}
		}
		fmt.Fprintf(&b, "- 边界：%s\n", tr.Boundary.Reason)
		if len(tr.Events) > 0 {
			b.WriteString("- 受控重建事件：\n")
			for _, ev := range tr.Events {
				fmt.Fprintf(&b, "  %d. `%s` [%s]\n", ev.Order, ev.Kind, ev.Severity)
				for _, action := range ev.Actions {
					fmt.Fprintf(&b, "     - %s\n", action)
				}
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}

func orEmpty(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
