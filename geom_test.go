package safereflow

import (
	"math"
	"testing"
)

func TestRectIntersectAndContains(t *testing.T) {
	a := Rect{X: 0, Y: 0, W: 100, H: 50}
	b := Rect{X: 50, Y: 25, W: 100, H: 50}
	got := a.Intersect(b)
	want := Rect{X: 50, Y: 25, W: 50, H: 25}
	if !rectNear(got, want) {
		t.Fatalf("intersection = %+v, want %+v", got, want)
	}
	if !a.Intersects(b) {
		t.Fatal("expected intersection")
	}
	if a.Intersect(Rect{X: 200, Y: 200, W: 10, H: 10}) != (Rect{}) {
		t.Fatal("expected empty intersection")
	}

	safe := Rect{X: 10, Y: 10, W: 100, H: 100}
	if !safe.ContainsRect(Rect{X: 20, Y: 20, W: 80, H: 80}) {
		t.Fatal("expected inner rect contained")
	}
	if safe.ContainsRect(Rect{X: 90, Y: 90, W: 30, H: 30}) {
		t.Fatal("overflowing rect must not be contained")
	}
}

func TestOverflowOf(t *testing.T) {
	safe := Rect{X: 100, Y: 100, W: 800, H: 600}
	box := Rect{X: 40, Y: 120, W: 900, H: 700}
	o := OverflowOf(box, safe)
	if !near(o.Left, 60) || !near(o.Right, 40) {
		t.Fatalf("horizontal overflow = %+v", o)
	}
	if !near(o.Top, 0) || !near(o.Bottom, 120) {
		t.Fatalf("vertical overflow = %+v", o)
	}
	if !o.Any() || !near(o.Max(), 120) {
		t.Fatalf("unexpected max overflow %+v", o)
	}
}

func TestMinimalTranslation(t *testing.T) {
	safe := Rect{X: 200, Y: 200, W: 1000, H: 500}

	// Off the left/bottom edges: push right and up.
	box := Rect{X: 100, Y: 300, W: 400, H: 360} // bottom 660 > 700? no: safe bottom 700; 300+360=660 fits
	dx, dy, fx, fy := MinimalTranslation(box, safe)
	if !fx || !fy {
		t.Fatal("expected both axes fit")
	}
	if !near(dx, 100) || !near(dy, 0) {
		t.Fatalf("dx,dy = %v,%v, want 100,0", dx, dy)
	}

	box2 := Rect{X: 300, Y: 260, W: 400, H: 480} // bottom 740 > safe bottom 700
	_, dy2, _, _ := MinimalTranslation(box2, safe)
	if !near(dy2, -40) {
		t.Fatalf("dy = %v, want -40", dy2)
	}

	tooWide := Rect{X: 0, Y: 0, W: 1400, H: 100}
	_, _, fitX, _ := MinimalTranslation(tooWide, safe)
	if fitX {
		t.Fatal("oversized width must report fitX=false")
	}
}

func rectNear(a, b Rect) bool {
	return near(a.X, b.X) && near(a.Y, b.Y) && near(a.W, b.W) && near(a.H, b.H)
}

func near(a, b float64) bool { return math.Abs(a-b) <= 1e-6 }
