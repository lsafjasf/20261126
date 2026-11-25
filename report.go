package vttlayout

import (
	"encoding/json"
	"fmt"
	"strings"
)

// CueReport is the per cue row in a JSON/text report.
type CueReport struct {
	Order    int     `json:"order"`
	Label    string  `json:"label"`
	ID       string  `json:"cue_id,omitempty"`
	Region   string  `json:"region,omitempty"`
	Start    string  `json:"start"`
	End      string  `json:"end"`
	Priority int     `json:"priority"`
	Lines    int     `json:"lines"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	W        float64 `json:"w"`
	H        float64 `json:"h"`
	Pushed   bool    `json:"pushed"`
	OK       bool    `json:"placement_ok"`
	Text     string  `json:"text"`
}

// Report is the stable JSON report shape emitted by the CLI.
type Report struct {
	Source      string       `json:"source,omitempty"`
	CanvasName  string       `json:"canvas"`
	Width       float64      `json:"width"`
	Height      float64      `json:"height"`
	Feasible    bool         `json:"feasible"`
	CueCount    int          `json:"cue_count"`
	RegionCount int          `json:"region_count"`
	BeforeCount int          `json:"authored_violation_count"`
	AfterCount  int          `json:"remaining_violation_count"`
	ComboCount  int          `json:"infeasible_combo_count"`
	Warnings    []string     `json:"parse_warnings,omitempty"`
	Cues        []CueReport  `json:"cues"`
	Violations  []Violation  `json:"violations_before"`
	Remaining   []Violation  `json:"violations_after"`
	Combos      []*Combo     `json:"combos"`
	Candidates  []*Candidate `json:"candidates"`
}

// NewReport converts a Plan into the stable report shape.
func NewReport(source string, doc *Document, plan *Plan) *Report {
	r := &Report{
		Source: source, CanvasName: plan.Canvas.Name,
		Width: plan.Canvas.Width, Height: plan.Canvas.Height,
		Feasible: plan.Feasible, CueCount: len(doc.Cues),
		RegionCount: len(doc.RegionOrder),
		BeforeCount: len(plan.Before), AfterCount: len(plan.After),
		ComboCount: len(plan.Combos),
		Violations: plan.Before, Remaining: plan.After,
		Combos: plan.Combos, Candidates: plan.Candidates,
	}
	for _, w := range doc.Warnings {
		r.Warnings = append(r.Warnings, w.Msg)
	}
	for _, b := range plan.Boxes {
		region := b.Cue.Settings.Region
		r.Cues = append(r.Cues, CueReport{
			Order: b.Cue.Order, Label: cueLabel(b.Cue), ID: b.Cue.ID,
			Region: region, Start: FormatTimestamp(b.Cue.StartMS),
			End: FormatTimestamp(b.Cue.EndMS), Priority: b.Priority,
			Lines: len(b.Lines), X: round1(b.Rect.X), Y: round1(b.Rect.Y),
			W: round1(b.Rect.W), H: round1(b.Rect.H), Pushed: b.Pushed,
			OK: b.PlacementOK, Text: b.Cue.Text,
		})
	}
	return r
}

// JSON renders an indented JSON report.
func (r *Report) JSON() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

func round1(v float64) float64 {
	return float64(int(v*10+sign(v)*0.5)) / 10
}

func sign(v float64) float64 {
	if v < 0 {
		return -1
	}
	return 1
}

// Text renders a concise Chinese console report.
func (r *Report) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "画布 %s %.0fx%.0f；cue=%d region=%d\n",
		r.CanvasName, r.Width, r.Height, r.CueCount, r.RegionCount)

	if len(r.Warnings) > 0 {
		b.WriteString("解析警告:\n")
		for _, w := range r.Warnings {
			fmt.Fprintf(&b, "  - %s\n", w)
		}
	}

	fmt.Fprintf(&b, "原始排布问题: %d（重叠/越界）\n", r.BeforeCount)
	for _, v := range r.Violations {
		other := "canvas"
		if v.CueB >= 0 {
			other = fmt.Sprintf("cue#%d", v.CueB+1)
		}
		fmt.Fprintf(&b, "  ! [%s] cue#%d vs %s: %s\n", v.Kind, v.CueA+1, other, v.Detail)
	}

	b.WriteString("重排结果:\n")
	for _, c := range r.Cues {
		state := "ok"
		if !c.OK {
			state = "无解"
		} else if c.Pushed {
			state = "已移动"
		}
		fmt.Fprintf(&b, "  #%d P%d 行%d [x%.0f y%.0f w%.0f h%.0f] %s %q\n",
			c.Order+1, c.Priority, c.Lines, c.X, c.Y, c.W, c.H, state, ellipsize(c.Text, 20))
	}

	if r.ComboCount == 0 {
		fmt.Fprintf(&b, "剩余冲突: %d；未发现无解组合；整体可行=%v\n", r.AfterCount, r.Feasible)
		return b.String()
	}

	for _, cb := range r.Combos {
		fmt.Fprintf(&b, "无解组合 %s @ %s-%s cues=%v 溢出 %.0fpx（需 %.0f / 可用 %.0f，认证=%v）\n",
			cb.ID, FormatTimestamp(cb.StartMS), FormatTimestamp(cb.EndMS),
			ordersPlusOne(cb.CueOrders), cb.OverflowPx, cb.TotalHeightPx,
			cb.AvailableHeightPx, cb.Certified)
		fmt.Fprintf(&b, "    %s\n", cb.Reason)
		for i, cand := range r.Candidates {
			if cand.ComboID != cb.ID {
				continue
			}
			mark := " "
			if cand.Resolves {
				mark = "✓"
			} else {
				mark = "×"
			}
			fmt.Fprintf(&b, "    %s #%d [%s] %s\n", mark, i+1, cand.Kind, cand.Title)
		}
	}
	fmt.Fprintf(&b, "整体可行=%v；剩余冲突=%d\n", r.Feasible, r.AfterCount)
	return b.String()
}

func ordersPlusOne(orders []int) []int {
	out := make([]int, len(orders))
	for i, o := range orders {
		out[i] = o + 1
	}
	return out
}
