package ttmlevents

import (
	"encoding/csv"
	"encoding/json"
	"io"
	"strconv"
	"strings"
)

// String returns the Markdown event table.
func (t *EventTable) String() string {
	return RenderMarkdown(t)
}

// RenderJSON writes the complete event table as indented JSON.
func RenderJSON(w io.Writer, table *EventTable) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(table)
}

// RenderEventCSV writes the flat causal event table as CSV.
func RenderEventCSV(w io.Writer, table *EventTable) error {
	cw := csv.NewWriter(w)
	header := []string{"sequence", "timeSeconds", "timecode", "phase", "kind", "target", "title", "detail", "causedBy", "orderKey"}
	if err := cw.Write(header); err != nil {
		return err
	}
	for _, e := range table.Events {
		record := []string{
			strconv.Itoa(e.Sequence),
			strconv.FormatFloat(e.TimeSeconds, 'f', 6, 64),
			e.Timecode, e.Phase, e.Kind, e.Target, e.Title, e.Detail, e.CausedBy, e.OrderKey,
		}
		if err := cw.Write(record); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// RenderMarkdown renders events, slices, text intervals, and style lanes.
func RenderMarkdown(table *EventTable) string {
	var b strings.Builder
	b.WriteString("# TTML 事件切片表\n\n")
	b.WriteString("- 排序规则：" + strings.Join(table.OrderingRules, "；") + "\n")
	b.WriteString("- 不同时间戳数：" + strconv.Itoa(table.UniqueTimestamps) + "\n")
	if len(table.Warnings) > 0 {
		b.WriteString("- 警告：" + strconv.Itoa(len(table.Warnings)) + " 条（见末尾）\n")
	}
	b.WriteString("\n")

	b.WriteString("## 因果事件表\n\n")
	b.WriteString("| # | t(s) | 时间 | 阶段 | 类型 | 目标 | 事件 | 原因/结果 |\n")
	b.WriteString("|---:|---:|---:|---|---|---|---|---|\n")
	for _, e := range table.Events {
		b.WriteString("| " + strings.Join([]string{
			strconv.Itoa(e.Sequence),
			strconv.FormatFloat(e.TimeSeconds, 'f', 3, 64),
			e.Timecode,
			phaseLabel(e.Phase),
			escapeCell(e.Kind),
			escapeCell(e.Target),
			escapeCell(e.Title),
			escapeCell(causeCell(e)),
		}, " | ") + " |\n")
	}

	b.WriteString("\n## 时间切片与可见文本\n\n")
	b.WriteString("| 切片 | 区间(s) | 文本 | 活动 runs |\n")
	b.WriteString("|---:|---|---|---:|\n")
	for _, s := range table.Slices {
		b.WriteString("| " + strings.Join([]string{
			strconv.Itoa(s.Index),
			intervalSeconds(s.BeginSeconds, s.EndSeconds, s.OpenEnded),
			escapeCell(displayText(s.Text)),
			strconv.Itoa(len(s.Runs)),
		}, " | ") + " |\n")
	}

	b.WriteString("\n## 所有文本区间\n\n")
	b.WriteString("| Run | 区间(s) | 可见 | 文本 | 基础样式 |\n")
	b.WriteString("|---|---|---|---|---|\n")
	for _, r := range table.Runs {
		if r.Whitespace || r.Kind == "break" {
			continue
		}
		b.WriteString("| " + strings.Join([]string{
			r.ID,
			intervalSeconds(r.BeginSeconds, r.EndSeconds, r.End == nil),
			boolCN(r.TimingVisible),
			escapeCell(displayText(r.Text)),
			escapeCell(formatProperties(r.Style)),
		}, " | ") + " |\n")
	}

	b.WriteString("\n## 样式区间\n\n")
	b.WriteString("| Run | 属性 | 区间(s) | 值 | 末值 | 插值 | 来源 |\n")
	b.WriteString("|---|---|---|---|---|---|---|\n")
	for _, st := range table.StyleIntervals {
		b.WriteString("| " + strings.Join([]string{
			st.RunID,
			escapeCell(st.Property),
			intervalSeconds(st.BeginSeconds, st.EndSeconds, st.OpenEnded),
			escapeCell(st.Value),
			escapeCell(st.EndValue),
			escapeCell(st.Interpolation),
			escapeCell(firstNonEmpty(st.AnimationPath, st.Source)),
		}, " | ") + " |\n")
	}

	if len(table.Warnings) > 0 {
		b.WriteString("\n## 警告\n\n")
		for _, w := range table.Warnings {
			b.WriteString("- `" + w.Code + "`")
			if w.Path != "" {
				b.WriteString(" " + escapeCell(w.Path))
			}
			b.WriteString("：" + escapeCell(w.Message) + "\n")
		}
	}
	return b.String()
}

func phaseLabel(phase string) string {
	switch phase {
	case PhaseEnd:
		return "结束/摘除"
	case PhaseBegin:
		return "开始/进入"
	case PhaseChange:
		return "样式变化"
	default:
		return phase
	}
}

func causeCell(e Event) string {
	prefix := ""
	if e.CauseSequence > 0 {
		prefix = "←#" + strconv.Itoa(e.CauseSequence) + " "
	}
	parts := make([]string, 0, 2)
	if strings.TrimSpace(e.Detail) != "" {
		parts = append(parts, e.Detail)
	}
	if strings.TrimSpace(e.CausedBy) != "" && e.CausedBy != e.Detail {
		parts = append(parts, e.CausedBy)
	}
	return prefix + strings.Join(parts, "；")
}

func escapeCell(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", " ⏎ ")
	s = strings.ReplaceAll(s, "\r", "")
	return s
}

func displayText(s string) string {
	s = strings.TrimSpace(s)
	return escapeCell(s)
}

func intervalSeconds(begin float64, end *float64, open bool) string {
	if end == nil || open {
		return "[" + strconv.FormatFloat(begin, 'f', 3, 64) + ", ∞)"
	}
	return "[" + strconv.FormatFloat(begin, 'f', 3, 64) + ", " + strconv.FormatFloat(*end, 'f', 3, 64) + ")"
}

func formatProperties(props []Property) string {
	parts := make([]string, 0, len(props))
	for _, p := range props {
		parts = append(parts, p.Name+"="+p.Value)
	}
	return strings.Join(parts, "; ")
}

func boolCN(v bool) string {
	if v {
		return "是"
	}
	return "否"
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
