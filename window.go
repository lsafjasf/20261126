package cea708

// WindowConfig is the declarative definition of a service window. It is the
// structured counterpart of the CEA-708 DFx (DefineWindow) command.
type WindowConfig struct {
	// ID is the window index 0..7.
	ID int
	// RowCount/ColCount are the text buffer size in cells. Zero values get
	// readable defaults.
	RowCount int
	ColCount int
	// AnchorRow/AnchorCol place the window's top-left cell on the composition
	// canvas. The real standard uses proportional anchor coordinates; cell
	// coordinates keep composition deterministic.
	AnchorRow int
	AnchorCol int
	// AnchorPoint/Priority are retained for fidelity with DFx/SWA.
	AnchorPoint int
	Priority    int
	// Visible defines the initial visibility bit carried by DFx.
	Visible bool
}

func (cfg WindowConfig) withDefaults() WindowConfig {
	if cfg.RowCount <= 0 {
		cfg.RowCount = 4
	}
	if cfg.ColCount <= 0 {
		cfg.ColCount = DefaultCols
	}
	if cfg.RowCount > DefaultRows {
		cfg.RowCount = DefaultRows
	}
	if cfg.ColCount > DefaultCols {
		cfg.ColCount = DefaultCols
	}
	if cfg.ID < 0 {
		cfg.ID = 0
	}
	if cfg.ID >= WindowCount {
		cfg.ID = WindowCount - 1
	}
	return cfg
}

// Window is one CEA-708 window: its definition, visibility and retained text.
//
// Hiding a window keeps its text (HDW/DSW semantics); deleting a window
// destroys both definition and contents (DLW semantics).
type Window struct {
	id      int
	defined bool
	visible bool
	cfg     WindowConfig

	rows int
	cols int
	text [][]rune // row-major, spaces for blanks

	penRow int
	penCol int
}

func newWindow(id int) Window {
	return Window{id: id}
}

// ID returns the window index 0..7.
func (w *Window) ID() int { return w.id }

// Defined reports whether DFx has created the window and DLW has not removed it.
func (w *Window) Defined() bool { return w.defined }

// Visible reports whether the window participates in composition.
func (w *Window) Visible() bool { return w.defined && w.visible }

// Config returns the current window configuration.
func (w *Window) Config() WindowConfig { return w.cfg }

// Lines returns the retained window text (trailing blanks trimmed).
func (w *Window) Lines() []string {
	if !w.defined {
		return nil
	}
	out := make([]string, w.rows)
	for r := 0; r < w.rows; r++ {
		last := -1
		for c := w.cols - 1; c >= 0; c-- {
			if w.text[r][c] != ' ' && w.text[r][c] != 0 {
				last = c
				break
			}
		}
		if last >= 0 {
			out[r] = string(w.text[r][:last+1])
		}
	}
	return out
}

func (w *Window) define(cfg WindowConfig) {
	cfg.ID = w.id
	cfg = cfg.withDefaults()
	w.cfg = cfg
	w.defined = true
	w.visible = cfg.Visible
	w.rows = cfg.RowCount
	w.cols = cfg.ColCount
	w.text = make([][]rune, w.rows)
	for r := range w.text {
		w.text[r] = make([]rune, w.cols)
		for c := range w.text[r] {
			w.text[r][c] = ' '
		}
	}
	w.penRow, w.penCol = 0, 0
}

// clear implements CLW: erase all text but keep definition, geometry,
// attributes and visibility. The pen returns to the origin.
func (w *Window) clear() {
	if !w.defined {
		return
	}
	for r := range w.text {
		for c := range w.text[r] {
			w.text[r][c] = ' '
		}
	}
	w.penRow, w.penCol = 0, 0
}

func (w *Window) show() {
	if w.defined {
		w.visible = true
	}
}
func (w *Window) hide() {
	if w.defined {
		w.visible = false
	}
}
func (w *Window) toggle() {
	if w.defined {
		w.visible = !w.visible
	}
}

// delete implements DLW: remove definition and retained contents entirely.
func (w *Window) delete() {
	w.defined = false
	w.visible = false
	w.text = nil
	w.rows, w.cols = 0, 0
	w.penRow, w.penCol = 0, 0
	w.cfg = WindowConfig{ID: w.id}
}

func (w *Window) setPenLocation(row, col int) {
	if !w.defined {
		return
	}
	if row < 0 {
		row = 0
	}
	if col < 0 {
		col = 0
	}
	if row >= w.rows {
		row = w.rows - 1
	}
	if col >= w.cols {
		col = w.cols - 1
	}
	w.penRow, w.penCol = row, col
}

func (w *Window) backspace() {
	if !w.defined {
		return
	}
	if w.penCol > 0 {
		w.penCol--
	} else if w.penRow > 0 {
		w.penRow--
		w.penCol = w.cols - 1
	}
	w.text[w.penRow][w.penCol] = ' '
}

// carriageReturn implements CR: move to column zero of the next row,
// scrolling the window when the bottom is reached.
func (w *Window) carriageReturn() {
	if !w.defined {
		return
	}
	w.penCol = 0
	w.penRow++
	if w.penRow >= w.rows {
		w.scrollUp()
		w.penRow = w.rows - 1
	}
}

// horizontalCarriageReturn implements HCR: column zero of the current row.
func (w *Window) horizontalCarriageReturn() {
	if !w.defined {
		return
	}
	w.penCol = 0
}

func (w *Window) scrollUp() {
	for r := 1; r < w.rows; r++ {
		copy(w.text[r-1], w.text[r])
	}
	last := w.rows - 1
	for c := 0; c < w.cols; c++ {
		w.text[last][c] = ' '
	}
}

// writeRune draws one glyph at the pen and advances, wrapping within the
// window buffer. Text directed at an undefined window is dropped, matching
// decoders that require a DFx before drawing.
func (w *Window) writeRune(r rune) bool {
	if !w.defined || r == 0 {
		return false
	}
	if r == '\n' {
		w.carriageReturn()
		return true
	}
	w.text[w.penRow][w.penCol] = r
	w.penCol++
	if w.penCol >= w.cols {
		w.penCol = 0
		w.penRow++
		if w.penRow >= w.rows {
			w.scrollUp()
			w.penRow = w.rows - 1
		}
	}
	return true
}

// rasterize stamps visible glyphs onto the composition canvas. The canvas is
// expected to have been cleared by the caller.
func (w *Window) rasterize(c *Canvas, owner ServiceID) {
	if !w.Defined() || !w.visible {
		return
	}
	for row := 0; row < w.rows; row++ {
		for col := 0; col < w.cols; col++ {
			ch := w.text[row][col]
			if ch == ' ' || ch == 0 {
				continue // transparent background
			}
			c.Set(w.cfg.AnchorRow+row, w.cfg.AnchorCol+col, Cell{
				Rune:   ch,
				Owner:  owner,
				Window: w.id,
			})
		}
	}
}
