// Package layout maps ASS script coordinates onto an output frame.
package layout

import (
	"fmt"
	"math"
	"strings"

	"assclip/ass"
)

// Strategy controls how the script canvas is fitted into the output frame.
type Strategy string

const (
	// Fit preserves aspect ratio and letterboxes/pillarboxes the canvas.
	Fit Strategy = "fit"
	// Fill preserves aspect ratio and crops the overflowing canvas.
	Fill Strategy = "fill"
	// Stretch maps script X/Y independently and changes the canvas aspect.
	Stretch Strategy = "stretch"
)

// ParseStrategy validates a command-line strategy name.
func ParseStrategy(value string) (Strategy, error) {
	switch Strategy(strings.ToLower(strings.TrimSpace(value))) {
	case Fit, "contain", "letterbox":
		return Fit, nil
	case Fill, "cover", "crop":
		return Fill, nil
	case Stretch, "scale":
		return Stretch, nil
	default:
		return "", fmt.Errorf("unknown strategy %q (use fit, fill, or stretch)", value)
	}
}

// Point is an output-space point.
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Rect is an output-space axis-aligned rectangle.
type Rect struct {
	MinX float64 `json:"min_x"`
	MinY float64 `json:"min_y"`
	MaxX float64 `json:"max_x"`
	MaxY float64 `json:"max_y"`
}

func (r Rect) Width() float64  { return r.MaxX - r.MinX }
func (r Rect) Height() float64 { return r.MaxY - r.MinY }
func (r Rect) Area() float64 {
	if r.Width() <= 0 || r.Height() <= 0 {
		return 0
	}
	return r.Width() * r.Height()
}

func (r Rect) Empty() bool { return r.MaxX <= r.MinX || r.MaxY <= r.MinY }

// Intersect returns the overlapping rectangle. The second result is false when
// the rectangles only touch or do not overlap.
func (r Rect) Intersect(other Rect) (Rect, bool) {
	out := Rect{
		MinX: math.Max(r.MinX, other.MinX),
		MinY: math.Max(r.MinY, other.MinY),
		MaxX: math.Min(r.MaxX, other.MaxX),
		MaxY: math.Min(r.MaxY, other.MaxY),
	}
	return out, !out.Empty()
}

// Contains reports whether other lies entirely inside r.
func (r Rect) Contains(other Rect) bool {
	return other.MinX >= r.MinX && other.MaxX <= r.MaxX &&
		other.MinY >= r.MinY && other.MaxY <= r.MaxY
}

// Canvas is the affine map from script coordinates to output pixels.
type Canvas struct {
	TargetWidth  int      `json:"target_width"`
	TargetHeight int      `json:"target_height"`
	ScriptWidth  float64  `json:"script_width"`
	ScriptHeight float64  `json:"script_height"`
	Strategy     Strategy `json:"strategy"`
	ScaleX       float64  `json:"scale_x"`
	ScaleY       float64  `json:"scale_y"`
	OffsetX      float64  `json:"offset_x"`
	OffsetY      float64  `json:"offset_y"`
}

// NewCanvas builds a mapping from a PlayRes canvas to a target output size.
func NewCanvas(scriptWidth, scriptHeight float64, targetWidth, targetHeight int, strategy Strategy) (Canvas, error) {
	if scriptWidth <= 0 || scriptHeight <= 0 {
		return Canvas{}, fmt.Errorf("script resolution must be positive, got %.3fx%.3f", scriptWidth, scriptHeight)
	}
	if targetWidth <= 0 || targetHeight <= 0 {
		return Canvas{}, fmt.Errorf("target resolution must be positive, got %dx%d", targetWidth, targetHeight)
	}
	c := Canvas{
		TargetWidth:  targetWidth,
		TargetHeight: targetHeight,
		ScriptWidth:  scriptWidth,
		ScriptHeight: scriptHeight,
		Strategy:     strategy,
	}
	tw, th := float64(targetWidth), float64(targetHeight)
	switch strategy {
	case Fit:
		s := math.Min(tw/scriptWidth, th/scriptHeight)
		c.ScaleX, c.ScaleY = s, s
		c.OffsetX = (tw - scriptWidth*s) / 2
		c.OffsetY = (th - scriptHeight*s) / 2
	case Fill:
		s := math.Max(tw/scriptWidth, th/scriptHeight)
		c.ScaleX, c.ScaleY = s, s
		c.OffsetX = (tw - scriptWidth*s) / 2
		c.OffsetY = (th - scriptHeight*s) / 2
	case Stretch:
		c.ScaleX = tw / scriptWidth
		c.ScaleY = th / scriptHeight
	default:
		return Canvas{}, fmt.Errorf("unknown strategy %q", strategy)
	}
	return c, nil
}

// Frame returns the output frame rectangle.
func (c Canvas) Frame() Rect {
	return Rect{MinX: 0, MinY: 0, MaxX: float64(c.TargetWidth), MaxY: float64(c.TargetHeight)}
}

// Content returns the mapped script canvas. With Fill it can extend beyond the
// output frame; with Fit it can be inset by letterboxing.
func (c Canvas) Content() Rect {
	return Rect{
		MinX: c.OffsetX,
		MinY: c.OffsetY,
		MaxX: c.OffsetX + c.ScriptWidth*c.ScaleX,
		MaxY: c.OffsetY + c.ScriptHeight*c.ScaleY,
	}
}

func (c Canvas) MapX(x float64) float64 { return c.OffsetX + x*c.ScaleX }
func (c Canvas) MapY(y float64) float64 { return c.OffsetY + y*c.ScaleY }

func (c Canvas) MapPoint(p ass.Point) Point {
	return Point{X: c.MapX(p.X), Y: c.MapY(p.Y)}
}

func (c Canvas) MapBounds(b ass.Bounds) Rect {
	return Rect{
		MinX: c.MapX(b.MinX),
		MinY: c.MapY(b.MinY),
		MaxX: c.MapX(b.MaxX),
		MaxY: c.MapY(b.MaxY),
	}
}

// MapInsets converts ASS margins. Horizontal margins use ScaleX and the
// vertical margin uses ScaleY, which matters for stretch/fill after a resolution
// change.
func (c Canvas) MapInsets(m Insets) Insets {
	return Insets{
		Left:     m.Left * math.Abs(c.ScaleX),
		Right:    m.Right * math.Abs(c.ScaleX),
		Vertical: m.Vertical * math.Abs(c.ScaleY),
	}
}

// Insets are ASS style/event margins in script units.
type Insets struct {
	Left     float64 `json:"left"`
	Right    float64 `json:"right"`
	Vertical float64 `json:"vertical"`
}

// ResolveInsets applies event-level overrides. ASS treats zero as "use style".
func ResolveInsets(style ass.Style, event ass.Event) Insets {
	in := Insets{
		Left:     style.MarginL,
		Right:    style.MarginR,
		Vertical: style.MarginV,
	}
	if event.MarginL != 0 {
		in.Left = event.MarginL
	}
	if event.MarginR != 0 {
		in.Right = event.MarginR
	}
	if event.MarginV != 0 {
		in.Vertical = event.MarginV
	}
	return in
}

// SafeArea converts ASS alignment semantics into an output-space area. For
// bottom/top alignment the vertical margin is anchored to that edge. For
// middle alignment ASS ignores MarginV, so the full frame height is available.
type SafeArea struct {
	Rect           Rect    `json:"rect"`
	Left           float64 `json:"left"`
	Right          float64 `json:"right"`
	Top            float64 `json:"top"`
	Bottom         float64 `json:"bottom"`
	VerticalAnchor string  `json:"vertical_anchor"`
}

func (c Canvas) SafeArea(insets Insets, alignment int) SafeArea {
	alignment = normalizeAlignment(alignment)
	px := c.MapInsets(insets)
	area := SafeArea{Left: px.Left, Right: px.Right}
	area.Rect.MinX = c.MapX(insets.Left)
	area.Rect.MaxX = c.MapX(c.ScriptWidth - insets.Right)
	switch {
	case alignment <= 3:
		area.VerticalAnchor = "bottom"
		area.Bottom = px.Vertical
		area.Top = 0
		area.Rect.MinY = c.MapY(0)
		area.Rect.MaxY = c.MapY(c.ScriptHeight - insets.Vertical)
	case alignment >= 7:
		area.VerticalAnchor = "top"
		area.Top = px.Vertical
		area.Bottom = 0
		area.Rect.MinY = c.MapY(insets.Vertical)
		area.Rect.MaxY = c.MapY(c.ScriptHeight)
	default:
		area.VerticalAnchor = "middle"
		area.Rect.MinY = c.MapY(0)
		area.Rect.MaxY = c.MapY(c.ScriptHeight)
	}
	return area
}

func normalizeAlignment(a int) int {
	if a < 1 || a > 9 {
		return 2
	}
	return a
}

// Overflow describes how far a rectangle extends past each frame edge.
type Overflow struct {
	Left   float64 `json:"left,omitempty"`
	Top    float64 `json:"top,omitempty"`
	Right  float64 `json:"right,omitempty"`
	Bottom float64 `json:"bottom,omitempty"`
}

func (o Overflow) Any() bool {
	return o.Left > 0 || o.Top > 0 || o.Right > 0 || o.Bottom > 0
}

// OutsideOverflow returns positive amounts for edges outside frame.
func OutsideOverflow(rect, frame Rect) Overflow {
	var o Overflow
	if frame.MinX-rect.MinX > 1e-6 {
		o.Left = frame.MinX - rect.MinX
	}
	if frame.MinY-rect.MinY > 1e-6 {
		o.Top = frame.MinY - rect.MinY
	}
	if rect.MaxX-frame.MaxX > 1e-6 {
		o.Right = rect.MaxX - frame.MaxX
	}
	if rect.MaxY-frame.MaxY > 1e-6 {
		o.Bottom = rect.MaxY - frame.MaxY
	}
	return o
}

// Clamp returns the part of rect inside frame.
func Clamp(rect, frame Rect) Rect {
	out, ok := rect.Intersect(frame)
	if !ok {
		return Rect{}
	}
	return out
}
