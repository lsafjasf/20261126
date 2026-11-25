package layout

import (
	"math"
	"testing"

	"assclip/ass"
)

func TestStrategyMapping(t *testing.T) {
	tests := []struct {
		name        string
		strategy    Strategy
		targetW     int
		targetH     int
		wantScaleX  float64
		wantScaleY  float64
		wantOffsetX float64
		wantOffsetY float64
	}{
		{"portrait fit", Fit, 1080, 1920, 0.5625, 0.5625, 0, 656.25},
		{"portrait fill", Fill, 1080, 1920, 1.7777777778, 1.7777777778, -1166.6666667, 0},
		{"portrait stretch", Stretch, 1080, 1920, 0.5625, 1.7777777778, 0, 0},
		{"ultrawide fit", Fit, 3440, 1440, 1.3333333333, 1.3333333333, 440, 0},
		{"ultrawide fill", Fill, 3440, 1440, 1.7916666667, 1.7916666667, 0, -247.5},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, err := NewCanvas(1920, 1080, tc.targetW, tc.targetH, tc.strategy)
			if err != nil {
				t.Fatal(err)
			}
			assertNear(t, "scale x", c.ScaleX, tc.wantScaleX)
			assertNear(t, "scale y", c.ScaleY, tc.wantScaleY)
			assertNear(t, "offset x", c.OffsetX, tc.wantOffsetX)
			assertNear(t, "offset y", c.OffsetY, tc.wantOffsetY)
		})
	}
}

func TestFitSafeAreaUsesMappedContentNotFrame(t *testing.T) {
	c, err := NewCanvas(1920, 1080, 1080, 1920, Fit)
	if err != nil {
		t.Fatal(err)
	}
	safe := c.SafeArea(Insets{Left: 100, Right: 100, Vertical: 50}, 2)
	if !nearRect(safe.Rect, Rect{MinX: 56.25, MinY: 656.25, MaxX: 1023.75, MaxY: 1235.625}) {
		t.Fatalf("safe rect = %+v", safe.Rect)
	}
	assertNear(t, "left margin", safe.Left, 56.25)
	assertNear(t, "bottom margin", safe.Bottom, 28.125)
	if safe.VerticalAnchor != "bottom" {
		t.Fatalf("anchor = %q", safe.VerticalAnchor)
	}
}

func TestSafeAreaTopAndMiddle(t *testing.T) {
	c, _ := NewCanvas(1920, 1080, 3440, 1440, Fit)
	insets := Insets{Left: 120, Right: 80, Vertical: 60}

	top := c.SafeArea(insets, 8)
	assertNear(t, "top min y", top.Rect.MinY, 80)
	assertNear(t, "top max y", top.Rect.MaxY, 1440)
	if top.VerticalAnchor != "top" {
		t.Fatalf("top anchor = %q", top.VerticalAnchor)
	}

	middle := c.SafeArea(insets, 5)
	assertNear(t, "middle min y", middle.Rect.MinY, 0)
	assertNear(t, "middle max y", middle.Rect.MaxY, 1440)
	if middle.VerticalAnchor != "middle" {
		t.Fatalf("middle anchor = %q", middle.VerticalAnchor)
	}
}

func TestMapBoundsAndOverflow(t *testing.T) {
	c, _ := NewCanvas(1920, 1080, 1080, 1920, Fit)
	got := c.MapBounds(ass.Bounds{MinX: 100, MinY: 700, MaxX: 1820, MaxY: 1000})
	if !nearRect(got, Rect{MinX: 56.25, MinY: 1050, MaxX: 1023.75, MaxY: 1218.75}) {
		t.Fatalf("mapped bounds = %+v", got)
	}
	overflow := OutsideOverflow(Rect{MinX: -10, MinY: -20, MaxX: 1100, MaxY: 1930}, c.Frame())
	assertNear(t, "left overflow", overflow.Left, 10)
	assertNear(t, "top overflow", overflow.Top, 20)
	assertNear(t, "right overflow", overflow.Right, 20)
	assertNear(t, "bottom overflow", overflow.Bottom, 10)
}

func TestResolveInsetsEventOverrides(t *testing.T) {
	style := ass.Style{MarginL: 100, MarginR: 200, MarginV: 300}
	event := ass.Event{MarginL: 0, MarginR: 50, MarginV: 0}
	got := ResolveInsets(style, event)
	if got.Left != 100 || got.Right != 50 || got.Vertical != 300 {
		t.Fatalf("insets = %+v", got)
	}
}

func assertNear(t *testing.T, label string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-6 {
		t.Fatalf("%s = %.10f, want %.10f", label, got, want)
	}
}

func nearRect(a, b Rect) bool {
	return math.Abs(a.MinX-b.MinX) < 1e-6 &&
		math.Abs(a.MinY-b.MinY) < 1e-6 &&
		math.Abs(a.MaxX-b.MaxX) < 1e-6 &&
		math.Abs(a.MaxY-b.MaxY) < 1e-6
}
