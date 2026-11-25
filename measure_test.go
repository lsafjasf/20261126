package safereflow

import (
	"strings"
	"testing"
)

func TestWrapCJKNeverExceedsWidth(t *testing.T) {
	m := NewGridMeasurer()
	const fontSize = 40
	text := strings.Repeat("安全区随镜头移动", 10) // 80 wide runes
	for _, width := range []float64{200, 480, 900, 1600} {
		lines := m.Wrap(text, fontSize, width)
		if len(lines) < 2 && width < float64(len([]rune(text)))*fontSize {
			t.Fatalf("width %v: expected wrapping", width)
		}
		for _, ln := range lines {
			if w := m.Advance(ln, fontSize); w > width+eps {
				t.Fatalf("width %v: line %q measured %.1f > %.1f", width, ln, w, width)
			}
		}
		if strings.Join(lines, "") != text {
			t.Fatalf("width %v: wrapping lost/duplicated runes:\n got %q\nwant %q", width, strings.Join(lines, ""), text)
		}
	}
}

func TestWrapLatinPrefersWords(t *testing.T) {
	m := NewGridMeasurer()
	lines := m.Wrap("the quick brown fox jumps", 40, 220)
	if len(lines) < 2 {
		t.Fatalf("expected multiple lines, got %q", lines)
	}
	for _, ln := range lines {
		if w := m.Advance(ln, 40); w > 220+eps {
			t.Fatalf("line %q too wide: %.1f", ln, w)
		}
		if strings.HasPrefix(ln, " ") || strings.HasSuffix(ln, " ") {
			t.Fatalf("line must be trimmed: %q", ln)
		}
	}
}

func TestLayoutAndAnchorBottomCenter(t *testing.T) {
	st := DefaultStyle()
	st.FontSize = 40
	l := LayoutText(NewGridMeasurer(), "两行\n文本", st, 400)
	if len(l.Lines) != 2 {
		t.Fatalf("lines=%v", l.Lines)
	}
	// Bottom-center anchor (960, 980), box width 200 -> x 860, bottom 980.
	box := boxAtAnchor(200, 100, AlignBottomCenter, 960, 980)
	if !near(box.X, 860) || !near(box.Y, 880) || !near(box.Bottom(), 980) {
		t.Fatalf("anchored box = %+v", box)
	}
}
