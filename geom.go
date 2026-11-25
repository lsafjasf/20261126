package safereflow

import (
	"fmt"
	"math"
)

// eps is the tolerance used for all geometry comparisons. Sub-pixel errors
// from float arithmetic must not turn an exactly-aligned cue into a violation.
const eps = 1e-7

// Rect is an axis-aligned rectangle in canvas pixel coordinates.
// X/Y are the top-left corner; W/H are width and height.
type Rect struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"width"`
	H float64 `json:"height"`
}

// Right returns the exclusive right edge coordinate.
func (r Rect) Right() float64 { return r.X + r.W }

// Bottom returns the exclusive bottom edge coordinate.
func (r Rect) Bottom() float64 { return r.Y + r.H }

// CenterX returns the horizontal center.
func (r Rect) CenterX() float64 { return r.X + r.W/2 }

// CenterY returns the vertical center.
func (r Rect) CenterY() float64 { return r.Y + r.H/2 }

// Empty reports whether the rectangle has no positive area.
func (r Rect) Empty() bool { return r.W <= eps || r.H <= eps }

// String renders the rectangle as [x,y w×h].
func (r Rect) String() string {
	return fmt.Sprintf("[%.1f,%.1f %.1fx%.1f]", r.X, r.Y, r.W, r.H)
}

// Area returns the rectangle area.
func (r Rect) Area() float64 {
	if r.W <= 0 || r.H <= 0 {
		return 0
	}
	return r.W * r.H
}

// approxEqual compares floats with the shared geometry tolerance.
func approxEqual(a, b float64) bool { return math.Abs(a-b) <= eps }

// Intersect returns the overlap of r and other. The returned rectangle is
// Empty when the rectangles do not overlap (edges touching count as empty
// zero-area intersection, which is fine for containment checks).
func (r Rect) Intersect(other Rect) Rect {
	x0 := math.Max(r.X, other.X)
	y0 := math.Max(r.Y, other.Y)
	x1 := math.Min(r.Right(), other.Right())
	y1 := math.Min(r.Bottom(), other.Bottom())
	if x1-x0 <= eps || y1-y0 <= eps {
		return Rect{}
	}
	return Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}
}

// Intersects reports whether the rectangles share positive area.
func (r Rect) Intersects(other Rect) bool {
	return !r.Intersect(other).Empty()
}

// ContainsRect reports whether other is fully inside r, edge tolerance aware.
func (r Rect) ContainsRect(other Rect) bool {
	if other.Empty() {
		return true
	}
	return other.X+eps >= r.X &&
		other.Y+eps >= r.Y &&
		other.Right() <= r.Right()+eps &&
		other.Bottom() <= r.Bottom()+eps
}

// Overflow describes how many pixels of a cue box stick out past each edge
// of the safe rectangle. Every component is clamped to be non-negative.
type Overflow struct {
	Left   float64 `json:"left"`
	Right  float64 `json:"right"`
	Top    float64 `json:"top"`
	Bottom float64 `json:"bottom"`
}

// Any reports whether at least one edge overflows.
func (o Overflow) Any() bool {
	return o.Left > eps || o.Right > eps || o.Top > eps || o.Bottom > eps
}

// Max returns the largest single-edge overflow in pixels.
func (o Overflow) Max() float64 {
	return math.Max(math.Max(o.Left, o.Right), math.Max(o.Top, o.Bottom))
}

// OverflowOf measures how far box extends outside safe on every edge.
func OverflowOf(box, safe Rect) Overflow {
	return Overflow{
		Left:   math.Max(0, safe.X-box.X),
		Right:  math.Max(0, box.Right()-safe.Right()),
		Top:    math.Max(0, safe.Y-box.Y),
		Bottom: math.Max(0, box.Bottom()-safe.Bottom()),
	}
}

// FitsInside reports whether box is fully contained by safe.
func FitsInside(box, safe Rect) bool {
	return !OverflowOf(box, safe).Any()
}

// MinimalTranslation returns the smallest (dx, dy) that moves box inside
// safe. It works independently per axis: if the box is larger than safe on an
// axis it cannot be made to fit there, and fitX/fitY report that.
func MinimalTranslation(box, safe Rect) (dx, dy float64, fitX, fitY bool) {
	fitX = box.W <= safe.W+eps
	fitY = box.H <= safe.H+eps
	if box.X < safe.X-eps {
		dx = safe.X - box.X
	} else if box.Right() > safe.Right()+eps {
		dx = safe.Right() - box.Right()
	}
	if box.Y < safe.Y-eps {
		dy = safe.Y - box.Y
	} else if box.Bottom() > safe.Bottom()+eps {
		dy = safe.Bottom() - box.Bottom()
	}
	return dx, dy, fitX, fitY
}

// Translate returns r shifted by dx/dy.
func (r Rect) Translate(dx, dy float64) Rect {
	r.X += dx
	r.Y += dy
	return r
}

// RoundToPixel snaps to integer pixel boundaries outward: origins are floored
// and far edges are ceiled, so the resulting box still contains r.
func (r Rect) RoundToPixel() Rect {
	x := math.Floor(r.X)
	y := math.Floor(r.Y)
	right := math.Ceil(r.Right())
	bottom := math.Ceil(r.Bottom())
	return Rect{X: x, Y: y, W: math.Max(0, right-x), H: math.Max(0, bottom-y)}
}

// rectIntersectionAll intersects every rectangle. The boolean is false when
// no rectangle was supplied.
func rectIntersectionAll(rects []Rect) (Rect, bool) {
	if len(rects) == 0 {
		return Rect{}, false
	}
	out := rects[0]
	for _, r := range rects[1:] {
		out = out.Intersect(r)
		if out.Empty() {
			return Rect{}, true
		}
	}
	return out, true
}
