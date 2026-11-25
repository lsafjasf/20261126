package safereflow

// ResolvedCue bundles a cue with its effective style, layout and box.
type ResolvedCue struct {
	Cue       *Cue
	Style     Style
	Alignment Alignment
	Layout    Layout
	Box       Rect
	// Anchor is the fixed resize point derived from Box + Alignment.
	Anchor Point
	// WrapWidth is the width used to lay the current text out.
	WrapWidth float64
}

// ResolveCue computes the authoritative box for c. An explicit c.Box wins for
// geometry, but text is still measured so rewrap plans know the metrics.
func ResolveCue(doc *CueDocument, c *Cue, m Measurer) ResolvedCue {
	st := doc.EffectiveStyle(c)
	align := normalizeAlign(st.Alignment)
	canvas := doc.Canvas

	wrapWidth := st.MaxWidth
	if c.Box != nil && c.Box.W > 0 {
		if inner := c.Box.W - 2*st.PaddingX; inner > 0 && (wrapWidth <= 0 || inner < wrapWidth) {
			wrapWidth = inner
		}
	}
	if wrapWidth <= 0 {
		wrapWidth = canvas.Width - 2*st.MarginX - 2*st.PaddingX
	}
	if wrapWidth < st.FontSize {
		wrapWidth = st.FontSize
	}

	layout := LayoutText(m, c.Text, st, wrapWidth)

	var box Rect
	if c.Box != nil && c.Box.W > 0 && c.Box.H > 0 {
		box = *c.Box
	} else {
		box = BoxForLayout(layout, st, canvas, align, nil)
	}

	return ResolvedCue{
		Cue:       c,
		Style:     st,
		Alignment: align,
		Layout:    layout,
		Box:       box,
		Anchor:    AnchorOf(box, align),
		WrapWidth: wrapWidth,
	}
}

// layoutCandidate lays text out at wrapWidth and keeps the original anchor
// point fixed, returning the proposed border box.
func (rc ResolvedCue) layoutCandidate(text string, wrapWidth float64, m Measurer) (Layout, Rect) {
	l := LayoutText(m, text, rc.Style, wrapWidth)
	w := l.ContentW + 2*rc.Style.PaddingX
	h := l.ContentH + 2*rc.Style.PaddingY
	box := boxAtAnchor(w, h, rc.Alignment, rc.Anchor.X, rc.Anchor.Y)
	return l, box
}
