package cea708

import "testing"

func populatedDecoder(t *testing.T) *Decoder {
	t.Helper()
	d := NewDecoder(1)
	d.DefineWindow(WindowConfig{ID: 0, RowCount: 2, ColCount: 20, AnchorRow: 1, AnchorCol: 1, Visible: true})
	d.WriteText("HELLO")
	return d
}

func TestCLWKeepsDefinitionErasesText(t *testing.T) {
	d := populatedDecoder(t)
	if got := d.Window(0).Lines(); len(got) == 0 {
		t.Fatal("setup: expected text")
	}
	d.ClearWindows(1 << 0)
	w := d.Window(0)
	if !w.Defined() || !w.Visible() {
		t.Fatal("CLW must preserve definition and visibility")
	}
	for _, line := range w.Lines() {
		if line != "" {
			t.Fatalf("CLW did not erase text: %q", w.Lines())
		}
	}
}

func TestHDWRetainsTextDSWRestores(t *testing.T) {
	d := populatedDecoder(t)
	d.HideWindows(0x01)
	if d.Window(0).Visible() {
		t.Fatal("HDW did not hide window 0")
	}
	if lines := d.Window(0).Lines(); len(lines) == 0 || lines[0] != "HELLO" {
		t.Fatalf("HDW must retain text, got %q", lines)
	}
	f := d.Frame(1, 0, FramePresent, DefaultCols, DefaultRows)
	if f.Contains("HELLO") {
		t.Fatal("hidden window must not be composited")
	}

	d.DisplayWindows(0x01)
	if !d.Window(0).Visible() {
		t.Fatal("DSW did not show window 0")
	}
	f = d.Frame(2, 0, FramePresent, DefaultCols, DefaultRows)
	if !f.Contains("HELLO") {
		t.Fatal("DSW did not restore retained text to composition")
	}
}

func TestTGWTogglesVisibility(t *testing.T) {
	d := populatedDecoder(t)
	d.ToggleWindows(0x01)
	if d.Window(0).Visible() {
		t.Fatal("TGW on visible window should hide it")
	}
	d.ToggleWindows(0x01)
	if !d.Window(0).Visible() {
		t.Fatal("second TGW should show it again")
	}
}

func TestDLWDestroysWindow(t *testing.T) {
	d := populatedDecoder(t)
	d.SetCurrentWindow(0)
	d.DeleteWindows(0x01)
	w := d.Window(0)
	if w.Defined() {
		t.Fatal("DLW must undefine the window")
	}
	if len(w.Lines()) != 0 {
		t.Fatal("DLW must discard retained text")
	}
	if d.CurrentWindow() != 0 {
		t.Fatalf("current window should fall back to 0 after DLW, got %d", d.CurrentWindow())
	}
	f := d.Frame(1, 0, FramePresent, DefaultCols, DefaultRows)
	if f.Contains("HELLO") {
		t.Fatal("deleted window must not composite")
	}
}

func TestRSTDeletesAllWindows(t *testing.T) {
	d := NewDecoder(1)
	for i := 0; i < 3; i++ {
		d.DefineWindow(WindowConfig{ID: i, RowCount: 1, ColCount: 8, Visible: true})
	}
	d.Reset()
	for i := 0; i < WindowCount; i++ {
		if d.Window(i).Defined() {
			t.Fatalf("RST left window %d defined", i)
		}
	}
	if _, ok := d.LatchedFrame(); ok {
		t.Fatal("RST must drop the latched frame")
	}
}

func TestMaskedCommandsOnlyTouchMaskedWindows(t *testing.T) {
	d := NewDecoder(1)
	d.DefineWindow(WindowConfig{ID: 0, RowCount: 1, ColCount: 8, Visible: true})
	d.DefineWindow(WindowConfig{ID: 1, RowCount: 1, ColCount: 8, Visible: true})
	d.HideWindows(0x01) // only window 0
	if !d.Window(1).Visible() || d.Window(0).Visible() {
		t.Fatal("mask bit leaked to another window")
	}
}
