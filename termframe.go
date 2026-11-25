package vttlayout

import (
	"fmt"
	"strings"
)

type termCell struct {
	ch    rune
	style string
	cont  bool
}

// RenderTerminalFrame draws a character cell preview frame. cols controls the
// width in terminal columns; row count follows the canvas aspect ratio with
// a 2:1 cell height/width correction. CJK runes occupy two columns.
func RenderTerminalFrame(plan *Plan, atMS Milliseconds, mode FrameMode, cols int, color bool) string {
	if cols < 40 {
		cols = 40
	}
	cv := plan.Canvas
	rows := int(float64(cv.Height) / float64(cv.Width) * float64(cols) / 2.0)
	if rows < 20 {
		rows = 20
	}
	sx := float64(cols) / cv.Width
	sy := float64(rows) / cv.Height

	grid := make([][]termCell, rows)
	for y := range grid {
		grid[y] = make([]termCell, cols)
		for x := range grid[y] {
			grid[y][x] = termCell{ch: ' '}
		}
	}

	put := func(x, y int, r rune, style string) {
		if y < 0 || y >= rows || x < 0 || x >= cols {
			return
		}
		w := 1
		if IsWideRune(r) {
			w = 2
		}
		if x+w > cols {
			return
		}
		grid[y][x] = termCell{ch: r, style: style}
		if w == 2 {
			grid[y][x+1] = termCell{ch: ' ', style: style, cont: true}
		}
	}

	fillRect := func(r Rect, style string) {
		x0 := int(r.X * sx)
		x1 := int(r.Right() * sx)
		y0 := int(r.Y * sy)
		y1 := int(r.Bottom() * sy)
		for y := y0; y < y1 && y < rows; y++ {
			for x := x0; x < x1 && x < cols; x++ {
				if y >= 0 && x >= 0 {
					grid[y][x] = termCell{ch: ' ', style: style}
				}
			}
		}
	}

	drawBorder := func(r Rect, style string) {
		x0 := int(r.X * sx)
		x1 := int(r.Right()*sx) - 1
		y0 := int(r.Y * sy)
		y1 := int(r.Bottom()*sy) - 1
		if x1 <= x0 || y1 <= y0 {
			return
		}
		for x := x0 + 1; x < x1; x++ {
			put(x, y0, '─', style)
			put(x, y1, '─', style)
		}
		for y := y0 + 1; y < y1; y++ {
			put(x0, y, '│', style)
			put(x1, y, '│', style)
		}
		put(x0, y0, '┌', style)
		put(x1, y0, '┐', style)
		put(x0, y1, '└', style)
		put(x1, y1, '┘', style)
	}

	type drawItem struct {
		r      Rect
		lines  []string
		stroke string
		fill   string
		label  string
		bad    bool
	}
	var items []drawItem

	if mode == FrameBefore {
		for _, ab := range plan.Authored {
			if !activeNow(ab.Cue, atMS) {
				continue
			}
			items = append(items, drawItem{
				r: ab.Rect, lines: ab.Lines,
				stroke: "\x1b[93m", fill: "\x1b[48;2;58;44;8m",
				label: fmt.Sprintf("P%d %s", ab.Priority, cueLabel(ab.Cue)),
			})
		}
	} else {
		for _, bx := range plan.Boxes {
			if !activeNow(bx.Cue, atMS) {
				continue
			}
			stroke := "\x1b[92m"
			fill := "\x1b[48;2;10;46;34m"
			if !bx.PlacementOK {
				stroke = "\x1b[91m"
				fill = "\x1b[48;2;56;16;16m"
			} else if bx.Pushed {
				stroke = "\x1b[96m"
				fill = "\x1b[48;2;12;44;56m"
			}
			items = append(items, drawItem{
				r: bx.Rect, lines: bx.Lines, stroke: stroke, fill: fill,
				label: fmt.Sprintf("P%d %s", bx.Priority, cueLabel(bx.Cue)),
				bad:   !bx.PlacementOK,
			})
		}
	}

	// Paint fills first, then text/borders on top.
	for _, it := range items {
		if color {
			fillRect(it.r, it.fill)
		}
	}
	for _, it := range items {
		style := ""
		if color {
			style = it.stroke
		}
		drawBorder(it.r, style)

		x0 := int((it.r.X + cv.PadX*0.4) * sx)
		y0 := int((it.r.Y + cv.PadY) * sy)
		textCols := int(it.r.W*sx) - 2
		if textCols < 4 {
			textCols = 4
		}
		termMeter := Meter{EmPx: 1, NonCJKRatio: 1, SpaceRatio: 1, WideRatio: 2}
		wrapped := WrapLines(strings.Join(it.lines, "\n"), float64(textCols), termMeter)
		lineRows := int(cv.LineHeightPx()*sy + 0.6)
		if lineRows < 1 {
			lineRows = 1
		}
		textStyle := ""
		if color {
			textStyle = it.fill + "\x1b[97m"
		}
		for li, line := range wrapped {
			yy := y0 + 1 + li*lineRows
			xx := x0 + 1
			for _, rn := range line {
				put(xx, yy, rn, textStyle)
				xx += int(termMeter.RuneAdvance(rn))
			}
		}
		// Label at top border.
		if it.label != "" {
			lx := int(it.r.X*sx) + 2
			ly := int(it.r.Y * sy)
			xx := lx
			labelStyle := style
			for _, rn := range it.label {
				put(xx, ly, rn, labelStyle)
				xx += int(termMeter.RuneAdvance(rn))
			}
		}
	}

	// Overlap hatches for before mode.
	if mode == FrameBefore {
		for _, v := range plan.Before {
			if v.Kind != ViolationOverlap {
				continue
			}
			r := v.Intersection
			x0 := int(r.X * sx)
			x1 := int(r.Right() * sx)
			y0 := int(r.Y * sy)
			y1 := int(r.Bottom() * sy)
			for y := y0; y < y1 && y < rows; y++ {
				for x := x0; x < x1 && x < cols; x += 2 {
					ch := '╳'
					st := ""
					if color {
						st = "\x1b[91m"
					}
					put(x, y, ch, st)
				}
			}
		}
	}

	var out strings.Builder
	title := fmt.Sprintf(" %s @ %s [%s] ", cv.Name, formatAt(atMS), mode)
	out.WriteString(title)
	out.WriteString("\n")
	reset := ""
	if color {
		reset = "\x1b[0m"
	}
	for y := 0; y < rows; y++ {
		cur := ""
		for x := 0; x < cols; x++ {
			c := grid[y][x]
			if c.cont {
				continue
			}
			if color && c.style != cur {
				out.WriteString(reset)
				if c.style != "" {
					out.WriteString(c.style)
				}
				cur = c.style
			}
			out.WriteRune(c.ch)
		}
		if color {
			out.WriteString(reset)
		}
		out.WriteString("\n")
	}
	return out.String()
}

func formatAt(at Milliseconds) string {
	if at < 0 {
		return "overview"
	}
	return FormatTimestamp(at)
}
