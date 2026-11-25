package safereflow

import (
	"strings"
	"unicode"
)

// Measurer estimates text advances and performs line wrapping. The default
// GridMeasurer is deterministic and font-free; production pipelines can plug
// a FreeType/HarfBuzz-backed implementation through Options.Measurer.
type Measurer interface {
	// Advance returns the width of s in canvas pixels at fontSize px.
	Advance(s string, fontSize float64) float64
	// Wrap breaks text (which may already contain '\n') into lines no wider
	// than maxWidth. A non-positive maxWidth means no wrapping.
	Wrap(text string, fontSize, maxWidth float64) []string
}

// GridMeasurer uses fixed em-advance estimates: wide scripts (CJK, Hangul,
// Kana, fullwidth forms) take 1.0 em, other printable runes 0.56 em and
// spaces 0.3 em. It is intentionally conservative for layout safety.
type GridMeasurer struct {
	NarrowAdvance float64 // em fraction for narrow runes; default 0.56
	SpaceAdvance  float64 // em fraction for spaces; default 0.30
	WideAdvance   float64 // em fraction for wide runes; default 1.00
}

// NewGridMeasurer returns a measurer with sensible advance defaults.
func NewGridMeasurer() GridMeasurer {
	return GridMeasurer{NarrowAdvance: 0.56, SpaceAdvance: 0.3, WideAdvance: 1.0}
}

func (m GridMeasurer) defaults() GridMeasurer {
	if m.NarrowAdvance <= 0 {
		m.NarrowAdvance = 0.56
	}
	if m.SpaceAdvance <= 0 {
		m.SpaceAdvance = 0.3
	}
	if m.WideAdvance <= 0 {
		m.WideAdvance = 1.0
	}
	return m
}

// IsWideRune reports whether r occupies a full em-square in typical CJK fonts.
func IsWideRune(r rune) bool {
	if unicode.Is(unicode.Han, r) ||
		unicode.Is(unicode.Hiragana, r) ||
		unicode.Is(unicode.Katakana, r) ||
		unicode.Is(unicode.Hangul, r) {
		return true
	}
	if r >= 0x1100 && r <= 0x115F { // Hangul Jamo
		return true
	}
	if r >= 0x2E80 && r <= 0x303E { // CJK radicals / symbols
		return true
	}
	if r >= 0x3041 && r <= 0x33FF { // Kana + CJK symbols
		return true
	}
	if r >= 0x3400 && r <= 0x4DBF { // Ext A
		return true
	}
	if r >= 0xF900 && r <= 0xFAFF { // compatibility ideographs
		return true
	}
	if r >= 0xFE30 && r <= 0xFE4F { // CJK compatibility forms
		return true
	}
	if r >= 0xFF00 && r <= 0xFF60 { // fullwidth ASCII
		return true
	}
	if r >= 0xFFE0 && r <= 0xFFE6 {
		return true
	}
	return false
}

// RuneAdvance returns the em fraction for one rune.
func (m GridMeasurer) RuneAdvance(r rune) float64 {
	m = m.defaults()
	switch {
	case r == '\t':
		return m.SpaceAdvance * 4
	case unicode.IsSpace(r):
		return m.SpaceAdvance
	case IsWideRune(r):
		return m.WideAdvance
	case !unicode.IsPrint(r):
		return 0
	default:
		return m.NarrowAdvance
	}
}

// Advance implements Measurer.
func (m GridMeasurer) Advance(s string, fontSize float64) float64 {
	if fontSize <= 0 {
		fontSize = DefaultStyle().FontSize
	}
	total := 0.0
	for _, r := range s {
		total += m.RuneAdvance(r) * fontSize
	}
	return total
}

// Wrap implements greedy wrapping with Latin word-boundary preference and
// per-rune breaking for wide scripts and overlong words.
func (m GridMeasurer) Wrap(text string, fontSize, maxWidth float64) []string {
	if maxWidth <= eps {
		return strings.Split(text, "\n")
	}
	var lines []string
	for _, para := range strings.Split(text, "\n") {
		lines = append(lines, wrapParagraph(m, para, fontSize, maxWidth)...)
	}
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

func wrapParagraph(m GridMeasurer, para string, fontSize, maxWidth float64) []string {
	runes := []rune(para)
	if len(runes) == 0 {
		return []string{""}
	}
	var lines []string
	lineStart := 0
	lineWidth := 0.0
	lastSpace := -1 // index in runes of last breakable space on current line

	flushTo := func(end int) {
		lines = append(lines, strings.TrimSpace(string(runes[lineStart:end])))
		lineStart = end
		lineWidth = 0
		lastSpace = -1
	}

	for i := 0; i < len(runes); i++ {
		r := runes[i]
		adv := m.RuneAdvance(r) * fontSize
		if lineWidth+adv > maxWidth+eps && i > lineStart {
			// Prefer breaking at the last space on this line.
			breakAt := i
			if lastSpace > lineStart {
				breakAt = lastSpace
			}
			flushTo(breakAt)
			// Skip the breaking space itself.
			if breakAt < len(runes) && unicode.IsSpace(runes[breakAt]) {
				lineStart = breakAt + 1
			} else {
				lineStart = breakAt
			}
			i = lineStart - 1
			lineWidth = 0
			lastSpace = -1
			continue
		}
		lineWidth += adv
		if unicode.IsSpace(r) {
			lastSpace = i
		}
	}
	if lineStart <= len(runes) {
		lines = append(lines, strings.TrimSpace(string(runes[lineStart:])))
	}
	return lines
}

// Layout is the result of measuring wrapped text.
type Layout struct {
	Lines      []string
	LineWidths []float64
	ContentW   float64
	ContentH   float64
	WrapWidth  float64
}

// Layout wraps and measures text using style metrics.
func LayoutText(m Measurer, text string, st Style, wrapWidth float64) Layout {
	if m == nil {
		m = NewGridMeasurer()
	}
	st = st.Normalize()
	if wrapWidth <= 0 {
		wrapWidth = st.MaxWidth
	}
	lines := m.Wrap(text, st.FontSize, wrapWidth)
	lineH := st.FontSize * st.LineHeight
	out := Layout{Lines: lines, WrapWidth: wrapWidth}
	for _, ln := range lines {
		w := m.Advance(ln, st.FontSize)
		out.LineWidths = append(out.LineWidths, w)
		if w > out.ContentW {
			out.ContentW = w
		}
	}
	out.ContentH = float64(len(lines)) * lineH
	return out
}

// BoxForLayout builds the border box (including padding) and anchors it.
func BoxForLayout(l Layout, st Style, canvas Canvas, align Alignment, anchor *Point) Rect {
	st = st.Normalize()
	w := l.ContentW + 2*st.PaddingX
	h := l.ContentH + 2*st.PaddingY
	var ax, ay float64
	if anchor != nil {
		ax, ay = anchor.X, anchor.Y
	} else {
		r := layoutAnchorRect(st, canvas)
		ax, ay = alignmentPoint(r, align)
	}
	return boxAtAnchor(w, h, align, ax, ay)
}

// Point is a 2D anchor coordinate.
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// layoutAnchorRect is the canvas rectangle after alignment margins.
func layoutAnchorRect(st Style, canvas Canvas) Rect {
	return Rect{
		X: st.MarginX,
		Y: st.MarginY,
		W: canvas.Width - 2*st.MarginX,
		H: canvas.Height - 2*st.MarginY,
	}
}

// alignmentPoint returns the anchor point of r for the 9-point alignment.
func alignmentPoint(r Rect, a Alignment) (float64, float64) {
	a = normalizeAlign(a)
	var x, y float64
	switch a {
	case AlignTopLeft, AlignMiddleLeft, AlignBottomLeft:
		x = r.X
	case AlignTopRight, AlignMiddleRight, AlignBottomRight:
		x = r.Right()
	default:
		x = r.CenterX()
	}
	switch a {
	case AlignTopLeft, AlignTopCenter, AlignTopRight:
		y = r.Y
	case AlignBottomLeft, AlignBottomCenter, AlignBottomRight:
		y = r.Bottom()
	default:
		y = r.CenterY()
	}
	return x, y
}

func normalizeAlign(a Alignment) Alignment {
	if !a.Valid() || a == "" {
		return AlignBottomCenter
	}
	return a
}

// boxAtAnchor returns a w×h box positioned so that its point described by
// align coincides with (ax, ay).
func boxAtAnchor(w, h float64, a Alignment, ax, ay float64) Rect {
	a = normalizeAlign(a)
	var x, y float64
	switch a {
	case AlignTopLeft, AlignMiddleLeft, AlignBottomLeft:
		x = ax
	case AlignTopRight, AlignMiddleRight, AlignBottomRight:
		x = ax - w
	default:
		x = ax - w/2
	}
	switch a {
	case AlignTopLeft, AlignTopCenter, AlignTopRight:
		y = ay
	case AlignBottomLeft, AlignBottomCenter, AlignBottomRight:
		y = ay - h
	default:
		y = ay - h/2
	}
	return Rect{X: x, Y: y, W: w, H: h}
}

// AnchorOf returns the fixed anchor point of an existing box for align.
func AnchorOf(box Rect, a Alignment) Point {
	ax, ay := alignmentPoint(box, normalizeAlign(a))
	return Point{X: ax, Y: ay}
}
