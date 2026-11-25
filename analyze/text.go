package analyze

import (
	"math"
	"strings"
	"unicode"

	"assclip/ass"
	"assclip/layout"
)

type visualSettings struct {
	Alignment int
	FontSize  float64
	ScaleX    float64
	ScaleY    float64
	Spacing   float64
	Outline   float64
	Shadow    float64
}

func resolveVisualSettings(style ass.Style, override ass.Override) visualSettings {
	v := visualSettings{
		Alignment: style.Alignment,
		FontSize:  style.FontSize,
		ScaleX:    style.ScaleX,
		ScaleY:    style.ScaleY,
		Spacing:   style.Spacing,
		Outline:   style.Outline,
		Shadow:    style.Shadow,
	}
	if v.Alignment < 1 || v.Alignment > 9 {
		v.Alignment = 2
	}
	if v.FontSize <= 0 {
		v.FontSize = 48
	}
	if v.ScaleX <= 0 {
		v.ScaleX = 100
	}
	if v.ScaleY <= 0 {
		v.ScaleY = 100
	}
	if override.HasAlignment {
		v.Alignment = override.Alignment
	}
	if override.FontSize != nil {
		v.FontSize = *override.FontSize
	}
	if override.ScaleX != nil {
		v.ScaleX = *override.ScaleX
	}
	if override.ScaleY != nil {
		v.ScaleY = *override.ScaleY
	}
	if override.Outline != nil {
		v.Outline = *override.Outline
	}
	if override.Shadow != nil {
		v.Shadow = *override.Shadow
	}
	return v
}

// estimateTextBounds returns a deliberately conservative, font-independent
// estimate. It exists to identify obvious crop/margin collisions; callers must
// not treat it as rasterized text metrics.
func estimateTextBounds(style ass.Style, override ass.Override, canvas layout.Canvas, insets layout.Insets) (layout.Rect, bool) {
	text := strings.TrimSpace(override.VisibleText)
	if text == "" {
		return layout.Rect{}, false
	}
	v := resolveVisualSettings(style, override)
	font := v.FontSize
	fx, fy := v.ScaleX/100, v.ScaleY/100

	lines := strings.Split(override.VisibleText, "\n")
	maxWidth := 0.0
	for _, line := range lines {
		w := emWidth(line)*font*fx + math.Max(0, float64(len([]rune(line))-1))*v.Spacing
		maxWidth = math.Max(maxWidth, w)
	}
	height := float64(len(lines)) * font * fy * 1.2
	border := math.Max(0, v.Outline)
	shadow := math.Abs(v.Shadow)
	width := maxWidth + 2*border + shadow
	height += 2*border + shadow
	if width <= 0 || height <= 0 {
		return layout.Rect{}, false
	}

	var box layout.Rect
	switch {
	case override.MoveFrom != nil && override.MoveTo != nil:
		a := placeTextBox(width, height, v.Alignment, canvas.ScriptWidth, canvas.ScriptHeight, insets, override.MoveFrom)
		b := placeTextBox(width, height, v.Alignment, canvas.ScriptWidth, canvas.ScriptHeight, insets, override.MoveTo)
		box = unionRect(a, b)
	case override.Position != nil:
		box = placeTextBox(width, height, v.Alignment, canvas.ScriptWidth, canvas.ScriptHeight, insets, override.Position)
	default:
		box = placeTextBox(width, height, v.Alignment, canvas.ScriptWidth, canvas.ScriptHeight, insets, nil)
	}
	return canvas.MapBounds(ass.Bounds{MinX: box.MinX, MinY: box.MinY, MaxX: box.MaxX, MaxY: box.MaxY}), true
}

func placeTextBox(width, height float64, alignment int, scriptW, scriptH float64, insets layout.Insets, anchor *ass.Point) layout.Rect {
	var x, y float64
	if anchor != nil {
		col := (alignment - 1) % 3
		switch col {
		case 0:
			x = anchor.X
		case 1:
			x = anchor.X - width/2
		default:
			x = anchor.X - width
		}
		switch {
		case alignment <= 3:
			y = anchor.Y - height
		case alignment >= 7:
			y = anchor.Y
		default:
			y = anchor.Y - height/2
		}
		return layout.Rect{MinX: x, MinY: y, MaxX: x + width, MaxY: y + height}
	}

	col := (alignment - 1) % 3
	switch col {
	case 0:
		x = insets.Left
	case 1:
		x = (scriptW - width) / 2
	default:
		x = scriptW - insets.Right - width
	}
	switch {
	case alignment <= 3:
		y = scriptH - insets.Vertical - height
	case alignment >= 7:
		y = insets.Vertical
	default:
		y = (scriptH - height) / 2
	}
	return layout.Rect{MinX: x, MinY: y, MaxX: x + width, MaxY: y + height}
}

func unionRect(a, b layout.Rect) layout.Rect {
	return layout.Rect{
		MinX: math.Min(a.MinX, b.MinX),
		MinY: math.Min(a.MinY, b.MinY),
		MaxX: math.Max(a.MaxX, b.MaxX),
		MaxY: math.Max(a.MaxY, b.MaxY),
	}
}

func emWidth(line string) float64 {
	var total float64
	for _, r := range line {
		total += runeWidth(r)
	}
	return total
}

func runeWidth(r rune) float64 {
	switch {
	case r == '\t':
		return 1
	case unicode.IsSpace(r):
		return 0.33
	case unicode.Is(unicode.Mn, r):
		return 0
	case unicode.Is(unicode.Han, r), unicode.Is(unicode.Hiragana, r),
		unicode.Is(unicode.Katakana, r), unicode.Is(unicode.Hangul, r):
		return 1
	case r >= 0x1F000:
		return 1
	case r >= 0x1100 && r <= 0x115F:
		return 1
	case unicode.IsUpper(r):
		return 0.66
	case unicode.IsLower(r):
		return 0.54
	case unicode.IsDigit(r):
		return 0.57
	case strings.ContainsRune(".,:;'|!", r):
		return 0.28
	case strings.ContainsRune("-_()[]/\\", r):
		return 0.38
	case unicode.IsPunct(r):
		return 0.35
	default:
		return 0.58
	}
}
