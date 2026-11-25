package vttlayout

import "math"

// Edge indicates which canvas edge a cue is anchored to; collision pushes
// away from that edge.
type Edge int

const (
	EdgeBottom Edge = iota
	EdgeTop
)

// AuthoredBox is a cue measured and positioned on a target canvas before
// repair.
type AuthoredBox struct {
	Cue      *Cue
	Region   *Region
	Rect     Rect
	Lines    []string
	Edge     Edge
	Priority int
	// WidthPct is the effective cue/region width in percent of canvas width.
	WidthPct float64
}

// EffectiveWidthPct resolves size/region/default width.
func EffectiveWidthPct(c *Cue, r *Region) float64 {
	if c.Settings.HasSize {
		return clampPct(c.Settings.SizePct)
	}
	if r != nil {
		return clampPct(r.WidthPct)
	}
	return 100
}

func clampPct(v float64) float64 {
	if v < 1 {
		return 1
	}
	if v > 100 {
		return 100
	}
	return v
}

// BuildAuthoredBoxes measures every cue at its authored width and resolves
// its anchor rectangle on cv.
func BuildAuthoredBoxes(doc *Document, cv Canvas) []*AuthoredBox {
	meter := DefaultMeter(cv.FontPx)
	boxes := make([]*AuthoredBox, 0, len(doc.Cues))
	for _, c := range doc.Cues {
		var r *Region
		if c.Settings.Region != "" {
			r = doc.Regions[c.Settings.Region]
		}
		widthPct := EffectiveWidthPct(c, r)
		width := widthPct / 100 * cv.Width
		innerW := cv.ContentWidth(width)
		lines := WrapLines(c.Text, innerW, meter)
		h := cv.BlockHeight(len(lines))
		rect, edge := ResolveRect(c, r, cv, width, h)
		boxes = append(boxes, &AuthoredBox{
			Cue: c, Region: r, Rect: rect, Lines: lines, Edge: edge,
			Priority: doc.Priority(c), WidthPct: widthPct,
		})
	}
	return boxes
}

// ResolveRect maps cue settings / region anchors to a pixel rectangle of
// given width and measured height.
func ResolveRect(c *Cue, r *Region, cv Canvas, w, h float64) (Rect, Edge) {
	edge := EdgeBottom

	// ---- Horizontal ----
	var x float64
	switch {
	case c.Settings.HasPosition:
		px := c.Settings.PositionPct / 100 * cv.Width
		alignFactor := positionAlignFactor(c.Settings)
		x = px - alignFactor*w
	case r != nil:
		vx := r.ViewportAnchorX / 100 * cv.Width
		x = vx - r.RegionAnchorX/100*w
	default:
		// Centered within the safe rectangle.
		x = cv.SafeRect().X + (cv.SafeRect().W-w)/2
	}

	// ---- Vertical ----
	var y float64
	switch {
	case c.Settings.HasLinePct:
		py := c.Settings.LinePct / 100 * cv.Height
		f := lineAlignFactor(c.Settings.LineAlign)
		y = py - f*h
		if c.Settings.LinePct < 50 {
			edge = EdgeTop
		}
	case c.Settings.HasLineNumber:
		lh := cv.LineHeightPx()
		n := c.Settings.LineNumber
		if n < 0 {
			// Negative line numbers count from the bottom edge.
			base := cv.Height - cv.Safe.Bottom + (n+1)*lh
			y = base - h
			edge = EdgeBottom
		} else {
			y = cv.Safe.Top + math.Max(0, n-1)*lh
			edge = EdgeTop
		}
	case r != nil:
		vy := r.ViewportAnchorY / 100 * cv.Height
		y = vy - r.RegionAnchorY/100*h
		if r.ViewportAnchorY < 50 {
			edge = EdgeTop
		}
	default:
		y = cv.Height - cv.Safe.Bottom - h
		edge = EdgeBottom
	}

	return Rect{X: x, Y: y, W: w, H: h}, edge
}

func positionAlignFactor(s CueSettings) float64 {
	switch s.PositionAlign {
	case "line-left", "start", "left":
		return 0
	case "line-right", "end", "right":
		return 1
	case "center", "middle":
		return 0.5
	}
	switch s.Align {
	case "left", "start":
		return 0
	case "right", "end":
		return 1
	default:
		return 0.5
	}
}

func lineAlignFactor(a string) float64 {
	switch a {
	case "start":
		return 0
	case "center", "middle":
		return 0.5
	case "end":
		return 1
	}
	return 0
}

// GapPx is the vertical breathing room between stacked cue boxes.
func GapPx(cv Canvas) float64 { return cv.FontPx * 0.35 }
