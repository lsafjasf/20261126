package framealign

import (
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
)

// Report 汇总一批事件在指定时间线下的对齐结果。
type Report struct {
	Timeline   Timeline
	Alignments []Alignment
}

// BuildReport 逐事件对齐并汇总。
func BuildReport(tl Timeline, events []Event) Report {
	r := Report{Timeline: tl}
	for _, ev := range events {
		r.Alignments = append(r.Alignments, tl.Align(ev))
	}
	return r
}

var candidateHeader = []string{
	"event", "kind", "src_fps", "src_frame", "src_tc",
	"exact_tl_frame", "exact", "policy", "tl_frame", "tl_tc",
	"err_frames", "err_ms",
}

// candidateRow 把一条候选展开为表行。
func (r Report) candidateRow(al Alignment, c Candidate) []string {
	ev := al.Event
	return []string{
		ev.Label,
		string(ev.Kind),
		ev.SourceFPS.Name,
		fmt.Sprintf("%d", ev.SourceFrame),
		FormatTimecode(ev.SourceFPS, ev.SourceFrame),
		formatRat(al.ExactFrame),
		fmt.Sprintf("%t", al.Exact),
		string(c.Policy),
		fmt.Sprintf("%d", c.Frame),
		FormatTimecode(r.Timeline.Rate, c.Frame),
		formatRat(c.ErrFrames),
		fmt.Sprintf("%+.3f", c.ErrMillis),
	}
}

// WriteCandidatesCSV 导出"事件 × 候选"全量误差表（精确命中的事件
// 三个策略等价，只写一行 policy=exact）。
func (r Report) WriteCandidatesCSV(w io.Writer) error {
	if err := writeCSVLine(w, candidateHeader); err != nil {
		return err
	}
	for _, al := range r.Alignments {
		if al.Exact {
			c, _ := al.Choose(PolicyNearest)
			c.Policy = "exact"
			if err := writeCSVLine(w, r.candidateRow(al, c)); err != nil {
				return err
			}
			continue
		}
		for _, c := range al.Candidates {
			if err := writeCSVLine(w, r.candidateRow(al, c)); err != nil {
				return err
			}
		}
	}
	return nil
}

// PolicySummary 是一种取整策略在所有事件上的误差统计。
type PolicySummary struct {
	Policy    Policy
	Events    int
	Adjusted  int     // 非精确命中、发生取舍的事件数
	MaxAbsMs  float64 // 最大绝对误差
	RMSMs     float64 // 均方根误差
	MeanAbsMs float64
}

// Summarize 对每种策略统计误差；精确命中事件误差为 0 且计入 Events。
func (r Report) Summarize() []PolicySummary {
	policies := []Policy{PolicyFloor, PolicyCeil, PolicyNearest}
	out := make([]PolicySummary, 0, len(policies))
	for _, p := range policies {
		s := PolicySummary{Policy: p}
		var sq float64
		for _, al := range r.Alignments {
			c, err := al.Choose(p)
			if err != nil {
				continue
			}
			s.Events++
			if !al.Exact {
				s.Adjusted++
			}
			a := math.Abs(c.ErrMillis)
			s.MeanAbsMs += a
			sq += c.ErrMillis * c.ErrMillis
			if a > s.MaxAbsMs {
				s.MaxAbsMs = a
			}
		}
		if s.Events > 0 {
			s.MeanAbsMs /= float64(s.Events)
			s.RMSMs = math.Sqrt(sq / float64(s.Events))
		}
		out = append(out, s)
	}
	return out
}

// WriteSummaryCSV 导出每种策略的汇总误差表。
func (r Report) WriteSummaryCSV(w io.Writer) error {
	if err := writeCSVLine(w, []string{
		"policy", "events", "adjusted", "max_abs_err_ms", "mean_abs_err_ms", "rms_err_ms",
	}); err != nil {
		return err
	}
	for _, s := range r.Summarize() {
		if err := writeCSVLine(w, []string{
			string(s.Policy),
			fmt.Sprintf("%d", s.Events),
			fmt.Sprintf("%d", s.Adjusted),
			fmt.Sprintf("%.3f", s.MaxAbsMs),
			fmt.Sprintf("%.3f", s.MeanAbsMs),
			fmt.Sprintf("%.3f", s.RMSMs),
		}); err != nil {
			return err
		}
	}
	return nil
}

// WriteMarkdown 导出人类可读的 Markdown 误差表（候选明细 + 策略汇总）。
func (r Report) WriteMarkdown(w io.Writer) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# 跨帧率落点误差表（时间线 %s fps）\n\n", r.Timeline.Rate.Name)
	b.WriteString("## 候选明细\n\n")
	writeMDRow(&b, candidateHeader)
	sep := make([]string, len(candidateHeader))
	for i := range sep {
		sep[i] = "---"
	}
	writeMDRow(&b, sep)
	for _, al := range r.Alignments {
		if al.Exact {
			c, _ := al.Choose(PolicyNearest)
			c.Policy = "exact"
			writeMDRow(&b, r.candidateRow(al, c))
			continue
		}
		for _, c := range al.Candidates {
			writeMDRow(&b, r.candidateRow(al, c))
		}
	}
	b.WriteString("\n## 策略汇总\n\n")
	writeMDRow(&b, []string{"policy", "events", "adjusted", "max_abs_err_ms", "mean_abs_err_ms", "rms_err_ms"})
	writeMDRow(&b, []string{"---", "---", "---", "---", "---", "---"})
	for _, s := range r.Summarize() {
		writeMDRow(&b, []string{
			string(s.Policy),
			fmt.Sprintf("%d", s.Events),
			fmt.Sprintf("%d", s.Adjusted),
			fmt.Sprintf("%.3f", s.MaxAbsMs),
			fmt.Sprintf("%.3f", s.MeanAbsMs),
			fmt.Sprintf("%.3f", s.RMSMs),
		})
	}
	b.WriteString("\n## 候选说明\n\n")
	b.WriteString("- floor：向早取整，落点不晚于创作意图点。\n")
	b.WriteString("- ceil：向晚取整，落点不早于创作意图点。\n")
	b.WriteString("- nearest：绝对误差最小；恰半帧时取偶数帧（round-half-to-even）。\n")
	_, err := io.WriteString(w, b.String())
	return err
}

func writeMDRow(b *strings.Builder, cols []string) {
	b.WriteString("| " + strings.Join(cols, " | ") + " |\n")
}

func writeCSVLine(w io.Writer, cols []string) error {
	for i, c := range cols {
		if strings.ContainsAny(c, ",\"\n") {
			c = "\"" + strings.ReplaceAll(c, "\"", "\"\"") + "\""
		}
		if i > 0 {
			if _, err := io.WriteString(w, ","); err != nil {
				return err
			}
		}
		if _, err := io.WriteString(w, c); err != nil {
			return err
		}
	}
	_, err := io.WriteString(w, "\n")
	return err
}

// formatRat 以 "整数" 或 "整数+分子/分母" 形式展示有理数帧位置。
func formatRat(r Rat) string {
	if r.IsInt() {
		return fmt.Sprintf("%d", r.Num())
	}
	if r.Num() < 0 {
		return "-" + formatRat(r.Abs())
	}
	whole := r.Floor()
	frac := r.Frac()
	if whole == 0 {
		return fmt.Sprintf("%d/%d", frac.Num(), frac.Den())
	}
	return fmt.Sprintf("%d+%d/%d", whole, frac.Num(), frac.Den())
}

// SortAlignments 按事件标签稳定排序（导出前可选调用）。
func (r *Report) SortAlignments() {
	sort.SliceStable(r.Alignments, func(i, j int) bool {
		return r.Alignments[i].Event.Label < r.Alignments[j].Event.Label
	})
}
