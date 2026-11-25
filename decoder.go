package cea708

import "fmt"

// Diagnostic describes a malformed or unsupported byte in an inbound command
// stream. Decoders stay panic-free and preserve all state before the fault.
type Diagnostic struct {
	Offset int    `json:"offset"`
	Opcode int    `json:"opcode"`
	Code   string `json:"code"`
	Detail string `json:"detail,omitempty"`
}

func (d Diagnostic) Error() string {
	return fmt.Sprintf("cea708: %s at offset %d (opcode 0x%02X): %s", d.Code, d.Offset, d.Opcode, d.Detail)
}

// Decoder holds the complete independent state of one CEA-708 service: eight
// windows, the current window and a latch of the last composited frame.
//
// A Decoder is safe for single-goroutine use; Manager serializes concurrent
// callers with one coarse lock.
type Decoder struct {
	id      ServiceID
	windows [WindowCount]Window
	current int

	// latched is the last full composition of this service. It stays valid
	// while the service is deselected, which is what makes switching back
	// lossless.
	latched *Frame

	diags []Diagnostic

	// Delays counts DLY units consumed (each unit = 100ms per CEA-708).
	DelayTenths int
}

// NewDecoder creates an empty service decoder.
func NewDecoder(id ServiceID) *Decoder {
	d := &Decoder{id: id, current: 0}
	for i := range d.windows {
		d.windows[i] = newWindow(i)
	}
	return d
}

// ID returns the service number.
func (d *Decoder) ID() ServiceID { return d.id }

// CurrentWindow returns the CWx index (-1 only after an exceptional reset).
func (d *Decoder) CurrentWindow() int { return d.current }

// Diagnostics returns accumulated parse diagnostics.
func (d *Decoder) Diagnostics() []Diagnostic {
	return append([]Diagnostic(nil), d.diags...)
}

// FirstDiagnostic returns the earliest malformed-code record, if any.
func (d *Decoder) FirstDiagnostic() (Diagnostic, bool) {
	if len(d.diags) == 0 {
		return Diagnostic{}, false
	}
	return d.diags[0], true
}

func (d *Decoder) addDiag(offset, opcode int, code, detail string) {
	d.diags = append(d.diags, Diagnostic{Offset: offset, Opcode: opcode, Code: code, Detail: detail})
}

// Window returns a pointer to window i (0..7).
func (d *Decoder) Window(i int) *Window {
	if i < 0 || i >= WindowCount {
		return nil
	}
	return &d.windows[i]
}

// SetCurrentWindow implements CW0..CW7. Selecting an undefined window is
// legal; subsequent SPA/SPC/SPL and text target it until a DFx/DLW event.
func (d *Decoder) SetCurrentWindow(i int) error {
	if i < 0 || i >= WindowCount {
		return fmt.Errorf("cea708: window index %d out of range", i)
	}
	d.current = i
	return nil
}

// DefineWindow implements DFx through the structured WindowConfig.
func (d *Decoder) DefineWindow(cfg WindowConfig) {
	if cfg.ID < 0 || cfg.ID >= WindowCount {
		cfg.ID = d.current
	}
	d.windows[cfg.ID].define(cfg)
	d.current = cfg.ID
}

func (d *Decoder) eachMasked(mask uint8, fn func(*Window)) {
	for i := 0; i < WindowCount; i++ {
		if mask&(1<<uint(i)) != 0 {
			fn(&d.windows[i])
		}
	}
}

// ClearWindows implements CLW: erase text of masked windows; definitions and
// visibility survive.
func (d *Decoder) ClearWindows(mask uint8) { d.eachMasked(mask, (*Window).clear) }

// DisplayWindows implements DSW: show masked windows (retained text returns).
func (d *Decoder) DisplayWindows(mask uint8) { d.eachMasked(mask, (*Window).show) }

// HideWindows implements HDW: hide masked windows without deleting text.
func (d *Decoder) HideWindows(mask uint8) { d.eachMasked(mask, (*Window).hide) }

// ToggleWindows implements TGW.
func (d *Decoder) ToggleWindows(mask uint8) { d.eachMasked(mask, (*Window).toggle) }

// DeleteWindows implements DLW: destroy masked windows. If the current window
// is deleted, current falls back to 0.
func (d *Decoder) DeleteWindows(mask uint8) {
	d.eachMasked(mask, (*Window).delete)
	if mask&(1<<uint(d.current)) != 0 {
		d.current = 0
	}
}

// Reset implements RST: delete every window and restore power-on defaults.
// The service identity is retained.
func (d *Decoder) Reset() {
	for i := range d.windows {
		d.windows[i].delete()
	}
	d.current = 0
	d.DelayTenths = 0
	d.latched = nil
}

// SetPenLocation implements SPL on the current window.
func (d *Decoder) SetPenLocation(row, col int) {
	if w := d.Window(d.current); w != nil {
		w.setPenLocation(row, col)
	}
}

// Backspace handles C0 BS on the current window.
func (d *Decoder) Backspace() {
	if w := d.Window(d.current); w != nil {
		w.backspace()
	}
}

// CarriageReturn handles C0 CR.
func (d *Decoder) CarriageReturn() {
	if w := d.Window(d.current); w != nil {
		w.carriageReturn()
	}
}

// HorizontalCarriageReturn handles C0 HCR.
func (d *Decoder) HorizontalCarriageReturn() {
	if w := d.Window(d.current); w != nil {
		w.horizontalCarriageReturn()
	}
}

// WriteText appends text to the current window. Glyphs sent before the window
// is defined are discarded (matching hardware decoders).
func (d *Decoder) WriteText(s string) {
	w := d.Window(d.current)
	if w == nil || !w.defined {
		return
	}
	for _, r := range s {
		w.writeRune(mapGraphic(r))
	}
}

// compose renders this service alone onto a fresh cleared canvas.
func (d *Decoder) compose(cols, rows int) *Canvas {
	c := NewCanvas(cols, rows)
	// Rasterize in id order so higher ids paint later; Priority could reorder
	// here but id order is deterministic and sufficient for the model.
	for i := range d.windows {
		d.windows[i].rasterize(c, d.id)
	}
	return c
}

// Frame builds an immutable snapshot of this service's screen.
func (d *Decoder) Frame(index int64, seq uint64, kind FrameKind, cols, rows int) Frame {
	canvas := d.compose(cols, rows)
	f := Frame{
		Index:     index,
		StreamSeq: seq,
		Service:   d.id,
		Kind:      kind,
		Width:     cols,
		Height:    rows,
		Lines:     canvas.TextRows(),
		OwnerGrid: canvas.OwnerGrid(),
	}
	return f
}

// latch stores f as the service's restorable screen.
func (d *Decoder) latchFrame(f Frame) {
	fc := f
	d.latched = &fc
}

// LatchedFrame returns the last composition, if any.
func (d *Decoder) LatchedFrame() (Frame, bool) {
	if d.latched == nil {
		return Frame{}, false
	}
	return *d.latched, true
}

// mapGraphic applies small CEA-708 G0/G1 overrides on top of Unicode.
func mapGraphic(r rune) rune {
	switch r {
	case 0x7F:
		return '♪' // CEA-708 G0 music note
	default:
		return r
	}
}
