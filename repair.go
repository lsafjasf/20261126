package vttlayout

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// CandidateKind labels a remediation proposal.
type CandidateKind string

const (
	// CandidateSplitCue is a "拆行" proposal: split one cue into two
	// sequential cues so fewer lines are visible at once.
	CandidateSplitCue CandidateKind = "split_cue"
	// CandidateShiftTime moves a cue into a free adjacent time slot.
	CandidateShiftTime CandidateKind = "shift_time"
	// CandidateExtendTime relocates a cue and extends its end / the timeline
	// so the cues can be presented sequentially at a readable pace.
	CandidateExtendTime CandidateKind = "extend_time"
	// CandidateExtendCanvas enlarges the available (safe) canvas height.
	CandidateExtendCanvas CandidateKind = "extend_canvas"
	// CandidateReduceFont shrinks the cue font until the combo fits.
	CandidateReduceFont CandidateKind = "reduce_font"
)

// ProposedCue is one replacement cue inside a candidate.
type ProposedCue struct {
	ReplacesOrder int          `json:"replaces_order,omitempty"`
	StartMS       Milliseconds `json:"start_ms"`
	EndMS         Milliseconds `json:"end_ms"`
	Text          string       `json:"text"`
}

// Candidate is a concrete, Go verified remediation for one Combo.
type Candidate struct {
	ComboID     string        `json:"combo_id"`
	Kind        CandidateKind `json:"kind"`
	TargetOrder int           `json:"target_order"`
	TargetLabel string        `json:"target_label"`
	Title       string        `json:"title"`
	Detail      string        `json:"detail"`
	// Resolves is set by re running the full Go layout checks on the
	// modified document.
	Resolves bool `json:"resolves"`

	ShiftMS       Milliseconds  `json:"shift_ms,omitempty"`
	ExtensionMS   Milliseconds  `json:"extension_ms,omitempty"`
	NewStartMS    Milliseconds  `json:"new_start_ms,omitempty"`
	NewEndMS      Milliseconds  `json:"new_end_ms,omitempty"`
	ExtraHeightPx float64       `json:"extra_height_px,omitempty"`
	NewFontPx     float64       `json:"new_font_px,omitempty"`
	BreakLines    []string      `json:"break_lines,omitempty"`
	Proposed      []ProposedCue `json:"proposed_cues,omitempty"`
}

// ReadingModel controls duration estimates for time candidates.
type ReadingModel struct {
	MaxCharsPerSec float64
	MinDwellMS     Milliseconds
	GapMS          Milliseconds
}

// DefaultReading is the default reading speed model.
func DefaultReading() ReadingModel {
	return ReadingModel{MaxCharsPerSec: 18, MinDwellMS: 1000, GapMS: 120}
}

// BuildCandidates generates and verifies remediation candidates for every
// detected combo.
func BuildCandidates(doc *Document, cv Canvas, opts Options, plan *Plan) []*Candidate {
	reading := DefaultReading()
	var out []*Candidate
	for _, combo := range plan.Combos {
		target := chooseTarget(doc, combo)
		if target == nil {
			continue
		}
		out = append(out, splitCandidate(doc, cv, opts, combo, target))
		if c := timeCandidate(doc, cv, opts, combo, target, reading, true); c != nil {
			out = append(out, c)
		}
		if c := timeCandidate(doc, cv, opts, combo, target, reading, false); c != nil {
			out = append(out, c)
		}
		out = append(out, canvasCandidate(cv, opts, combo))
		if c := fontCandidate(doc, cv, opts, combo); c != nil {
			out = append(out, c)
		}
	}
	return out
}

func chooseTarget(doc *Document, combo *Combo) *Cue {
	byOrder := map[int]*Cue{}
	for _, c := range doc.Cues {
		byOrder[c.Order] = c
	}
	var choices []*Cue
	for _, o := range combo.CueOrders {
		if c, ok := byOrder[o]; ok {
			choices = append(choices, c)
		}
	}
	if len(choices) == 0 {
		return nil
	}
	sort.Slice(choices, func(i, j int) bool {
		pi, pj := doc.Priority(choices[i]), doc.Priority(choices[j])
		if pi != pj {
			return pi < pj // lowest priority first
		}
		li := len([]rune(choices[i].Text))
		lj := len([]rune(choices[j].Text))
		if li != lj {
			return li > lj // longest first
		}
		return choices[i].Order < choices[j].Order
	})
	return choices[0]
}

func cueLabel(c *Cue) string {
	if c.ID != "" {
		return c.ID
	}
	return fmt.Sprintf("cue#%d", c.Order+1)
}

// verifyWindow re runs layout on a modified document and reports whether the
// conflict intersecting [wStart,wEnd) has disappeared.
func verifyWindow(doc2 *Document, cv Canvas, opts Options, wStart, wEnd Milliseconds) bool {
	p := Layout(doc2, cv, opts)
	combos := DetectCombos(doc2, cv, p)
	for _, cb := range combos {
		if cb.StartMS < wEnd && cb.EndMS > wStart {
			return false
		}
	}
	for _, v := range p.After {
		if v.CueA >= 0 {
			var ca *Cue
			for _, c := range doc2.Cues {
				if c.Order == v.CueA {
					ca = c
				}
			}
			if ca != nil && ca.StartMS < wEnd && ca.EndMS > wStart {
				return false
			}
		}
		if v.CueB >= 0 {
			var cb *Cue
			for _, c := range doc2.Cues {
				if c.Order == v.CueB {
					cb = c
				}
			}
			if cb != nil && cb.StartMS < wEnd && cb.EndMS > wStart {
				return false
			}
		}
	}
	return true
}

func cloneDocShallow(doc *Document) *Document {
	return &Document{
		Regions: doc.Regions, RegionOrder: doc.RegionOrder,
		Cues:     append([]*Cue(nil), doc.Cues...),
		Warnings: doc.Warnings,
	}
}

func withMovedCue(doc *Document, target *Cue, ns, ne Milliseconds) *Document {
	d2 := cloneDocShallow(doc)
	for i, c := range d2.Cues {
		if c.Order == target.Order {
			cp := *c
			cp.StartMS, cp.EndMS = ns, ne
			d2.Cues[i] = &cp
		}
	}
	return d2
}

func withSplitCue(doc *Document, target *Cue, ts Milliseconds, textA, textB string) *Document {
	d2 := &Document{Regions: doc.Regions, RegionOrder: doc.RegionOrder}
	maxOrder := 0
	for _, c := range doc.Cues {
		if c.Order > maxOrder {
			maxOrder = c.Order
		}
		if c.Order == target.Order {
			continue
		}
		d2.Cues = append(d2.Cues, c)
	}
	a := *target
	a.StartMS, a.EndMS = target.StartMS, ts
	a.Text = textA
	b := *target
	b.StartMS, b.EndMS = ts, target.EndMS
	b.Text = textB
	b.Order = maxOrder + 1
	d2.Cues = append(d2.Cues, &a, &b)
	reindexOrders(d2)
	return d2
}

func reindexOrders(doc *Document) {
	sort.SliceStable(doc.Cues, func(i, j int) bool {
		if doc.Cues[i].StartMS != doc.Cues[j].StartMS {
			return doc.Cues[i].StartMS < doc.Cues[j].StartMS
		}
		return doc.Cues[i].Order < doc.Cues[j].Order
	})
	for i, c := range doc.Cues {
		c.Order = i
	}
}

// ---- Split (拆行) ----

func splitCandidate(doc *Document, cv Canvas, opts Options, combo *Combo, target *Cue) *Candidate {
	runes := []rune(target.Text)
	idx, boundary := FindSplitPoint(target.Text)
	textA := strings.TrimSpace(string(runes[:idx]))
	textB := strings.TrimSpace(string(runes[idx:]))

	dur := target.Duration()
	ratio := 0.5
	if total := len([]rune(textA)) + len([]rune(textB)); total > 0 {
		ratio = float64(len([]rune(textA))) / float64(total)
	}
	ts := target.StartMS + Milliseconds(math.Round(float64(dur)*ratio))
	if ts <= target.StartMS {
		ts = target.StartMS + dur/2
	}
	if ts >= target.EndMS {
		ts = target.StartMS + dur/2
	}

	d2 := withSplitCue(doc, target, ts, textA, textB)
	resolves := verifyWindow(d2, cv, opts, combo.StartMS, combo.EndMS)

	safe := cv.SafeRect()
	preview := WrapLines(target.Text, safe.W-2*cv.PadX, DefaultMeter(cv.FontPx))

	c := &Candidate{
		ComboID: combo.ID, Kind: CandidateSplitCue, TargetOrder: target.Order,
		TargetLabel: cueLabel(target),
		Title:       fmt.Sprintf("拆行：将 %s 在%s处拆为两条分时字幕", cueLabel(target), boundary),
		Detail: fmt.Sprintf(
			"拆点位于第 %d 个字符附近；上段 %q，下段 %q。拆分点 %s。原 safe width 下视觉行数 %d。",
			idx, ellipsize(textA, 24), ellipsize(textB, 24),
			FormatTimestamp(ts), len(preview)),
		Resolves:   resolves,
		BreakLines: preview,
		Proposed: []ProposedCue{
			{ReplacesOrder: target.Order, StartMS: target.StartMS, EndMS: ts, Text: textA},
			{StartMS: ts, EndMS: target.EndMS, Text: textB},
		},
	}
	return c
}

// FindSplitPoint returns the rune index and a Chinese description of the
// chosen linguistic boundary closest to the middle.
func FindSplitPoint(text string) (int, string) {
	runes := []rune(text)
	n := len(runes)
	if n == 0 {
		return 0, "中点"
	}
	best := n / 2
	bestCost := math.MaxFloat64
	bestDesc := "长度中点"
	for i := 1; i < n; i++ {
		r := runes[i-1]
		penalty, desc, ok := boundaryPenalty(r)
		if !ok {
			// Western word interior: not a legal break.
			continue
		}
		mid := math.Abs(float64(i-n/2)) / float64(n)
		cost := mid + penalty
		if cost < bestCost {
			bestCost = cost
			best = i
			bestDesc = desc
		}
	}
	if best <= 0 || best >= n {
		best = n / 2
	}
	return best, bestDesc
}

func boundaryPenalty(r rune) (float64, string, bool) {
	switch r {
	case '。', '！', '？', '!', '?':
		return 0, "句末标点", true
	case '；', ';':
		return 0.08, "分号", true
	case '，', ',', '、':
		return 0.18, "逗号停顿", true
	case '：', ':':
		return 0.22, "冒号", true
	case ' ':
		return 0.3, "空格", true
	case '\n':
		return 0, "已有换行", true
	}
	if IsWideRune(r) {
		return 0.45, "汉字边界", true
	}
	return 1, "", false
}

func ellipsize(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
