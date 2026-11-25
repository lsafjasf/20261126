package cea708

import (
	"strings"
	"testing"
)

func testWindow(id, row, col int, visible bool) WindowConfig {
	return WindowConfig{
		ID: id, RowCount: 3, ColCount: 24,
		AnchorRow: row, AnchorCol: col, Visible: visible,
	}
}

// TestSwitchToUnknownServicePresentsBlank proves that switching to a service
// which has not drawn anything yet never leaves the old service's glyphs on
// screen: the switch frame is blank, not an overlay.
func TestSwitchToUnknownServicePresentsBlank(t *testing.T) {
	m := NewManager(WithSelectedService(1))

	m.Update(1, func(d *Decoder) {
		d.DefineWindow(testWindow(0, 1, 1, true))
		d.WriteText("S1-ALPHA-MARKER")
	})
	if f, ok := m.LastFrame(); !ok || !f.Contains("S1-ALPHA-MARKER") {
		t.Fatalf("service 1 text not presented: %+v ok=%v", f, ok)
	}

	switched := m.SelectService(2)
	if switched.Service != 2 || switched.Kind != FrameSwitch {
		t.Fatalf("unexpected switch frame: %+v", switched)
	}
	if switched.Contains("S1-ALPHA-MARKER") {
		t.Fatalf("ghost text from service 1 in switch frame:\n%s", renderFrame(switched))
	}
	if lines := switched.NonEmptyLines(); len(lines) != 0 {
		t.Fatalf("switch to empty service must be blank, got %v", lines)
	}

	events := m.ClearEvents()
	if len(events) != 1 || events[0].Reason != ClearServiceSwitch ||
		events[0].From != 1 || events[0].To != 2 {
		t.Fatalf("expected one 1->2 switch clear, got %+v", events)
	}
}

// TestNoOverlapAcrossSwitch drives the full 1 -> 2 -> 1 sequence and scans the
// entire presented timeline for cross-service "叠字".
func TestNoOverlapAcrossSwitch(t *testing.T) {
	m := NewManager(WithSelectedService(1))

	m.Update(1, func(d *Decoder) {
		d.DefineWindow(testWindow(0, 2, 2, true))
		d.WriteText("ONE-ONLY-GLYPHS")
	})
	m.SelectService(2)
	m.Update(2, func(d *Decoder) {
		d.DefineWindow(testWindow(0, 2, 2, true))
		d.WriteText("TWO-ONLY-GLYPHS")
	})
	m.Update(2, func(d *Decoder) {
		d.CarriageReturn()
		d.WriteText("TWO-SECOND-ROW")
	})
	m.SelectService(1)

	frames := m.Frames()
	if len(frames) < 5 {
		t.Fatalf("expected a populated timeline, got %d frames", len(frames))
	}

	// Locate the first 1->2 switch frame.
	switchIdx := -1
	for i, f := range frames {
		if f.Kind == FrameSwitch && f.Service == 2 {
			switchIdx = i
			break
		}
	}
	if switchIdx < 0 {
		t.Fatal("missing service-2 switch frame")
	}

	// Every frame while service 2 is selected must contain only service-2
	// glyphs; service 1's marker must never bleed through.
	for i := switchIdx; ; i++ {
		if i >= len(frames) {
			t.Fatal("did not find switch back to service 1")
		}
		f := frames[i]
		if f.Kind == FrameSwitch && f.Service == 1 {
			break
		}
		if f.Service != 2 {
			t.Fatalf("frame %d tagged service %d in service-2 region", i, f.Service)
		}
		if f.Contains("ONE-ONLY-GLYPHS") {
			t.Fatalf("overlap: service-1 text in service-2 frame %d:\n%s", i, renderFrame(f))
		}
		for r, row := range f.OwnerGrid {
			for c, owner := range row {
				if owner != NoService && owner != 2 {
					t.Fatalf("frame %d cell (%d,%d) owned by service %d", i, r, c, owner)
				}
			}
		}
	}

	back, ok := m.LatchedScreen(1)
	if !ok || !back.Contains("ONE-ONLY-GLYPHS") {
		t.Fatalf("service 1 latch lost while deselected: %+v", back)
	}
	last := frames[len(frames)-1]
	if last.Service != 1 || !last.Contains("ONE-ONLY-GLYPHS") || last.Contains("TWO-") {
		t.Fatalf("switch back did not restore clean service 1 screen:\n%s", renderFrame(last))
	}
}

// TestServicesHaveIndependentScreens proves writes/clears in one service do
// not mutate the other service's windows.
func TestServicesHaveIndependentScreens(t *testing.T) {
	m := NewManager()
	m.Update(1, func(d *Decoder) {
		d.DefineWindow(testWindow(0, 0, 0, true))
		d.WriteText("S1-INDEPENDENT")
	})
	m.Update(2, func(d *Decoder) {
		d.DefineWindow(testWindow(0, 5, 5, true))
		d.WriteText("S2-INDEPENDENT")
	})

	s1, _ := m.LatchedScreen(1)
	s2, _ := m.LatchedScreen(2)
	if !s1.Contains("S1-INDEPENDENT") || s1.Contains("S2-INDEPENDENT") {
		t.Fatalf("service 1 screen contaminated:\n%s", renderFrame(s1))
	}
	if !s2.Contains("S2-INDEPENDENT") || s2.Contains("S1-INDEPENDENT") {
		t.Fatalf("service 2 screen contaminated:\n%s", renderFrame(s2))
	}

	// Clearing service 1 leaves service 2 intact.
	m.Update(1, func(d *Decoder) { d.ClearWindows(1 << 0) })
	s1b, _ := m.LatchedScreen(1)
	s2b, _ := m.LatchedScreen(2)
	if s1b.Contains("S1-INDEPENDENT") {
		t.Fatal("CLW did not erase service 1 text")
	}
	if !s2b.Contains("S2-INDEPENDENT") {
		t.Fatal("clearing service 1 mutated service 2")
	}
}

func renderFrame(f Frame) string {
	var b strings.Builder
	for _, line := range f.Lines {
		b.WriteString("|")
		b.WriteString(line)
		b.WriteString("|\n")
	}
	return b.String()
}
