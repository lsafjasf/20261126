package vttlayout

import (
	"fmt"
	"strings"
	"unicode"
)

// SafeArea are pixel insets reserved around the usable canvas (bezel,
// player controls, platform UI).
type SafeArea struct {
	Top    float64
	Right  float64
	Bottom float64
	Left   float64
}

// Canvas describes the target video/view port in pixels.
type Canvas struct {
	Name             string
	Width            float64
	Height           float64
	Safe             SafeArea
	FontPx           float64
	LineHeightFactor float64
	PadX             float64
	PadY             float64
}

// LineHeightPx is the used line pitch.
func (c Canvas) LineHeightPx() float64 { return c.FontPx * c.LineHeightFactor }

// ContentWidth is the region inner width after horizontal padding.
func (c Canvas) ContentWidth(regionWidth float64) float64 {
	return regionWidth - 2*c.PadX
}

// SafeRect returns the usable canvas after safe area insets.
func (c Canvas) SafeRect() Rect {
	return Rect{
		X: c.Safe.Left,
		Y: c.Safe.Top,
		W: c.Width - c.Safe.Left - c.Safe.Right,
		H: c.Height - c.Safe.Top - c.Safe.Bottom,
	}
}

// Landscape1080p is the authoring canvas used by the flagship example.
func Landscape1080p() Canvas {
	return Canvas{
		Name: "landscape-1080p", Width: 1920, Height: 1080,
		Safe:             SafeArea{Top: 48, Right: 96, Bottom: 108, Left: 96},
		FontPx:           54,
		LineHeightFactor: 1.3,
		PadX:             26, PadY: 12,
	}
}

// Portrait1080x1920 is the vertical-switch target: narrower width, taller
// stack, larger bottom control inset. The font stays height relative (the
// WebVTT UA stylesheet sizes cues in vh units), so it is large in coded
// pixels while line wrapping happens against the narrower width.
func Portrait1080x1920() Canvas {
	return Canvas{
		Name: "portrait-1080x1920", Width: 1080, Height: 1920,
		Safe:             SafeArea{Top: 110, Right: 72, Bottom: 250, Left: 72},
		FontPx:           92,
		LineHeightFactor: 1.3,
		PadX:             30, PadY: 14,
	}
}

// Portrait720x1280 is a smaller phone preset.
func Portrait720x1280() Canvas {
	c := Portrait1080x1920()
	s := 720.0 / 1080.0
	return Canvas{
		Name: "portrait-720x1280", Width: 720, Height: 1280,
		Safe: SafeArea{
			Top: c.Safe.Top * s, Right: c.Safe.Right * s,
			Bottom: c.Safe.Bottom * s, Left: c.Safe.Left * s,
		},
		FontPx: c.FontPx * s, LineHeightFactor: c.LineHeightFactor,
		PadX: c.PadX * s, PadY: c.PadY * s,
	}
}

// CanvasByName resolves a preset name. Unknown names yield false.
func CanvasByName(name string) (Canvas, bool) {
	switch strings.ToLower(name) {
	case "portrait", "portrait-1080x1920", "vertical":
		return Portrait1080x1920(), true
	case "portrait-720x1280", "portrait720":
		return Portrait720x1280(), true
	case "landscape", "landscape-1080p", "horizontal":
		return Landscape1080p(), true
	}
	return Canvas{}, false
}

// CustomCanvas builds a WxH canvas using the same proportional metrics as
// the presets: height relative font and proportional safe insets.
func CustomCanvas(w, h float64) Canvas {
	font := h * 0.048
	return Canvas{
		Name: fmt.Sprintf("custom-%.0fx%.0f", w, h), Width: w, Height: h,
		Safe: SafeArea{
			Top: h * 0.058, Right: w * 0.067,
			Bottom: h * 0.13, Left: w * 0.067,
		},
		FontPx: font, LineHeightFactor: 1.3,
		PadX: font * 0.34, PadY: font * 0.15,
	}
}

// Rect is an axis aligned rectangle in canvas pixels, Y grows downward.
type Rect struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

// Right returns X+W.
func (r Rect) Right() float64 { return r.X + r.W }

// Bottom returns Y+H.
func (r Rect) Bottom() float64 { return r.Y + r.H }

// Intersects reports strict positive area intersection.
func (r Rect) Intersects(o Rect) bool {
	return r.X < o.Right() && r.Right() > o.X &&
		r.Y < o.Bottom() && r.Bottom() > o.Y
}

// Intersection returns the overlap rectangle (W/H zero when disjoint).
func (r Rect) Intersection(o Rect) Rect {
	x1 := maxFloat(r.X, o.X)
	y1 := maxFloat(r.Y, o.Y)
	x2 := minFloat(r.Right(), o.Right())
	y2 := minFloat(r.Bottom(), o.Bottom())
	if x2 <= x1 || y2 <= y1 {
		return Rect{}
	}
	return Rect{X: x1, Y: y1, W: x2 - x1, H: y2 - y1}
}

// ContainsRect reports whether o is fully inside r.
func (r Rect) ContainsRect(o Rect) bool {
	return o.X >= r.X-0.5 && o.Y >= r.Y-0.5 &&
		o.Right() <= r.Right()+0.5 && o.Bottom() <= r.Bottom()+0.5
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// Meter estimates text advances without depending on an external font.
// CJK and full width runes are 1em, other glyphs use a compact proportional
// model. The model is deterministic so line counts are reproducible in tests.
type Meter struct {
	EmPx        float64
	NonCJKRatio float64
	SpaceRatio  float64
	WideRatio   float64
}

// DefaultMeter builds the meter matching a canvas font size.
func DefaultMeter(fontPx float64) Meter {
	return Meter{EmPx: fontPx, NonCJKRatio: 0.58, SpaceRatio: 0.32, WideRatio: 1.0}
}

// RuneAdvance is the estimated advance width in pixels.
func (m Meter) RuneAdvance(r rune) float64 {
	switch {
	case r == '\t':
		return m.EmPx * m.SpaceRatio * 4
	case unicode.IsSpace(r):
		return m.EmPx * m.SpaceRatio
	case IsWideRune(r):
		return m.EmPx * m.WideRatio
	case unicode.IsUpper(r):
		return m.EmPx * (m.NonCJKRatio + 0.12)
	case unicode.IsDigit(r):
		return m.EmPx * (m.NonCJKRatio + 0.02)
	default:
		return m.EmPx * m.NonCJKRatio
	}
}

// Measure returns the estimated width of s.
func (m Meter) Measure(s string) float64 {
	w := 0.0
	for _, r := range s {
		w += m.RuneAdvance(r)
	}
	return w
}

// IsWideRune reports CJK / Hangul / Kana / full width ranges that allow a
// line break on either side and consume one full em.
func IsWideRune(r rune) bool {
	switch {
	case r >= 0x1100 && r <= 0x115F: // Hangul Jamo
		return true
	case r >= 0x2E80 && r <= 0x303E: // CJK radicals / Kangxi
		return true
	case r >= 0x3041 && r <= 0x33FF: // Kana / CJK symbols
		return true
	case r >= 0x3400 && r <= 0x4DBF:
		return true
	case r >= 0x4E00 && r <= 0x9FFF:
		return true
	case r >= 0xA000 && r <= 0xA4CF: // Yi
		return true
	case r >= 0xAC00 && r <= 0xD7A3: // Hangul syllables
		return true
	case r >= 0xF900 && r <= 0xFAFF:
		return true
	case r >= 0xFE30 && r <= 0xFE4F:
		return true
	case r >= 0xFF00 && r <= 0xFF60: // fullwidth forms
		return true
	case r >= 0xFFE0 && r <= 0xFFE6:
		return true
	case r >= 0x20000 && r <= 0x2FFFD:
		return true
	}
	return false
}

// visualToken is a wrapping unit.
type visualToken struct {
	text  string
	width float64
	glue  bool // spaces: collapsible, line break allowed after
	wide  bool // CJK rune: line break allowed before/after
}

// WrapLines greedily wraps text to maxWidth, honoring explicit newlines,
// spaces and CJK break opportunities. Overlong western words are hard split.
func WrapLines(text string, maxWidth float64, m Meter) []string {
	if maxWidth <= 0 {
		return splitExplicit(text)
	}
	var out []string
	for _, para := range strings.Split(text, "\n") {
		out = append(out, wrapParagraph(para, maxWidth, m)...)
	}
	if len(out) == 0 {
		out = []string{""}
	}
	return out
}

func splitExplicit(text string) []string {
	parts := strings.Split(text, "\n")
	if len(parts) == 0 {
		return []string{""}
	}
	return parts
}

func tokenizeParagraph(para string, m Meter) []visualToken {
	var tokens []visualToken
	var word strings.Builder
	wordW := 0.0
	flushWord := func() {
		if word.Len() > 0 {
			s := word.String()
			tokens = append(tokens, visualToken{text: s, width: wordW})
			word.Reset()
			wordW = 0
		}
	}
	for _, r := range para {
		switch {
		case IsWideRune(r):
			flushWord()
			tokens = append(tokens, visualToken{
				text: string(r), width: m.RuneAdvance(r), wide: true,
			})
		case unicode.IsSpace(r):
			flushWord()
			tokens = append(tokens, visualToken{
				text: " ", width: m.RuneAdvance(r), glue: true,
			})
		default:
			word.WriteRune(r)
			wordW += m.RuneAdvance(r)
		}
	}
	flushWord()
	return tokens
}

func wrapParagraph(para string, maxWidth float64, m Meter) []string {
	tokens := tokenizeParagraph(para, m)
	var lines []string
	var cur strings.Builder
	curW := 0.0

	newLine := func() {
		lines = append(lines, strings.TrimRight(cur.String(), " "))
		cur.Reset()
		curW = 0
	}

	for ti := 0; ti < len(tokens); ti++ {
		tok := tokens[ti]

		// Collapse leading glue on a fresh line.
		if curW == 0 && tok.glue {
			continue
		}

		if curW+tok.width <= maxWidth {
			cur.WriteString(tok.text)
			curW += tok.width
			continue
		}

		// Overflow. Glue just wraps away.
		if tok.glue {
			newLine()
			continue
		}

		// A western word longer than a full line: hard split rune wise.
		if !tok.wide && tok.width > maxWidth && curW == 0 {
			remaining := tok.text
			for len(remaining) > 0 {
				var piece strings.Builder
				pieceW := 0.0
				consumed := 0
				for _, r := range remaining {
					rw := m.RuneAdvance(r)
					if pieceW+rw > maxWidth && piece.Len() > 0 {
						break
					}
					piece.WriteRune(r)
					pieceW += rw
					consumed += len(string(r))
				}
				lines = append(lines, piece.String())
				remaining = remaining[consumed:]
			}
			continue
		}

		// Regular break: start a new line. CJK tokens may begin the line;
		// otherwise drop a separating space.
		newLine()
		if tok.glue {
			continue
		}
		cur.WriteString(tok.text)
		curW = tok.width
	}
	if cur.Len() > 0 || len(lines) == 0 {
		lines = append(lines, strings.TrimRight(cur.String(), " "))
	}
	return lines
}

// BlockHeight computes the box height for n visual lines.
func (c Canvas) BlockHeight(nLines int) float64 {
	if nLines < 1 {
		nLines = 1
	}
	return float64(nLines)*c.LineHeightPx() + 2*c.PadY
}
