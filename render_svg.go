package vttlayout

import (
	"fmt"
	"html"
	"strings"
)

// FrameMode selects authored ("before") or solved ("after") geometry.
type FrameMode string

const (
	FrameBefore FrameMode = "before"
	FrameAfter  FrameMode = "after"
)

// RenderSVGFrame renders one portrait canvas frame at time atMS. When
// atMS is negative, every cue is shown (overview mode).
func RenderSVGFrame(plan *Plan, atMS Milliseconds, mode FrameMode) string {
	cv := plan.Canvas
	var b strings.Builder
	const hud = 96.0
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 -%.0f %.0f %.0f" font-family="%s">`,
		int(cv.Width/2), int((cv.Height+hud)/2), hud, cv.Width, cv.Height+hud,
		`'Noto Sans CJK SC','Source Han Sans SC','PingFang SC','Microsoft YaHei',sans-serif`)

	fmt.Fprintf(&b, `<rect x="0" y="-%.0f" width="%.0f" height="%.0f" fill="#070a10"/>`, hud, cv.Width, cv.Height+hud)
	// Video/canvas background.
	fmt.Fprintf(&b, `<rect x="0" y="0" width="%.0f" height="%.0f" fill="#0b0f17"/>`, cv.Width, cv.Height)

	// Safe area.
	safe := cv.SafeRect()
	fmt.Fprintf(&b, `<rect x="%.0f" y="%.0f" width="%.0f" height="%.0f" rx="18" fill="none" stroke="#3b82f6" stroke-width="3" stroke-dasharray="16 12" opacity="0.75"/>`,
		safe.X, safe.Y, safe.W, safe.H)
	fmt.Fprintf(&b, `<text x="%.0f" y="%.0f" fill="#60a5fa" font-size="30" text-anchor="end">safe area</text>`,
		safe.Right()-6, safe.Y-16)

	switch mode {
	case FrameBefore:
		renderBeforeSVG(&b, plan, atMS)
	default:
		renderAfterSVG(&b, plan, atMS)
	}

	// Timecode HUD.
	tc := "overview"
	if atMS >= 0 {
		tc = FormatTimestamp(atMS)
	}
	modeLabel := "重排后 after"
	accent := "#10b981"
	if mode == FrameBefore {
		modeLabel = "原始 before"
		accent = "#f59e0b"
	}
	fmt.Fprintf(&b, `<rect x="24" y="-72" width="560" height="60" rx="14" fill="#000" opacity="0.55"/>`)
	fmt.Fprintf(&b, `<text x="48" y="-31" fill="#e5e7eb" font-size="38" font-family="monospace">%s</text>`, tc)
	fmt.Fprintf(&b, `<text x="%.0f" y="-31" fill="%s" font-size="34" text-anchor="end">%s</text>`,
		cv.Width-40, accent, modeLabel)

	b.WriteString(`</svg>`)
	return b.String()
}

func activeNow(c *Cue, atMS Milliseconds) bool {
	return atMS < 0 || (c.StartMS <= atMS && c.EndMS > atMS)
}

func renderBeforeSVG(b *strings.Builder, plan *Plan, atMS Milliseconds) {
	active := map[int]bool{}
	for _, ab := range plan.Authored {
		if !activeNow(ab.Cue, atMS) {
			continue
		}
		active[ab.Cue.Order] = true
		drawTextBox(b, ab.Rect, ab.Lines, plan.Canvas, boxStyle{
			fill: "rgba(245,158,11,0.16)", stroke: "#f59e0b",
			label: fmt.Sprintf("P%d %s", ab.Priority, cueLabel(ab.Cue)),
		})
	}
	// Highlight pairwise intersections.
	for _, v := range plan.Before {
		if v.Kind != ViolationOverlap || v.CueB < 0 {
			continue
		}
		if !active[v.CueA] || !active[v.CueB] {
			continue
		}
		r := v.Intersection
		fmt.Fprintf(b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="rgba(239,68,68,0.5)" stroke="#ef4444" stroke-width="2"/>`,
			r.X, r.Y, r.W, r.H)
	}
}

func renderAfterSVG(b *strings.Builder, plan *Plan, atMS Milliseconds) {
	for _, bx := range plan.Boxes {
		if !activeNow(bx.Cue, atMS) {
			continue
		}
		style := boxStyle{
			fill: "rgba(16,185,129,0.14)", stroke: "#10b981",
			label: fmt.Sprintf("P%d %s", bx.Priority, cueLabel(bx.Cue)),
		}
		if !bx.PlacementOK {
			style.fill = "rgba(239,68,68,0.16)"
			style.stroke = "#ef4444"
			style.label += " 无解"
		} else if bx.Pushed {
			style.stroke = "#38bdf8"
			style.fill = "rgba(56,189,248,0.12)"
		}
		drawTextBox(b, bx.Rect, bx.Lines, plan.Canvas, style)
	}
}

type boxStyle struct {
	fill   string
	stroke string
	label  string
}

func drawTextBox(b *strings.Builder, r Rect, lines []string, cv Canvas, st boxStyle) {
	fmt.Fprintf(b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" rx="16" fill="%s" stroke="%s" stroke-width="4"/>`,
		r.X, r.Y, r.W, r.H, st.fill, st.stroke)
	// Priority / cue label above the box.
	fmt.Fprintf(b, `<text x="%.1f" y="%.1f" fill="%s" font-size="26">%s</text>`,
		r.X+6, r.Y-12, st.stroke, escapeSVG(st.label))

	lh := cv.LineHeightPx()
	baseline := r.Y + cv.PadY + cv.FontPx*0.82
	tx := r.X + cv.PadX
	for i, line := range lines {
		lineY := baseline + float64(i)*lh
		fmt.Fprintf(b, `<text x="%.1f" y="%.1f" fill="#f8fafc" font-size="%.1f">%s</text>`,
			tx, lineY, cv.FontPx, escapeSVG(line))
	}
}

func escapeSVG(s string) string {
	return html.EscapeString(s)
}

func fmtFloat(v float64) string {
	return fmt.Sprintf("%.1f", v)
}

// KeyFrameTimes returns one midpoint time per distinct active set.
func KeyFrameTimes(doc *Document) []Milliseconds {
	stages := BuildStages(doc.Cues)
	seen := map[string]bool{}
	var out []Milliseconds
	for _, s := range stages {
		key := s.Key()
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, (s.StartMS+s.EndMS)/2)
	}
	if len(out) == 0 && len(doc.Cues) > 0 {
		out = append(out, doc.Cues[0].StartMS)
	}
	return out
}

// RichestFrameTime returns the midpoint of the stage with the most active
// cues, useful as a representative preview frame.
func RichestFrameTime(doc *Document) Milliseconds {
	stages := BuildStages(doc.Cues)
	best := Milliseconds(-1)
	bestN := -1
	for _, s := range stages {
		if len(s.Cues) > bestN {
			bestN = len(s.Cues)
			best = (s.StartMS + s.EndMS) / 2
		}
	}
	if best < 0 && len(doc.Cues) > 0 {
		return doc.Cues[0].StartMS
	}
	return best
}
