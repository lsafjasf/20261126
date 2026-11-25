package cea708

// ServiceID identifies a CEA-708 caption service. Valid on-air services are
// 1..63; service 0 is used as the "no owner" marker by composited cells.
//
// Each service keeps a fully independent decoder/window/screen state.
type ServiceID uint8

// NoService marks canvas cells that are currently blank background.
const NoService ServiceID = 0

// The two services used by the reported program.
const (
	Service1 ServiceID = 1
	Service2 ServiceID = 2
)

// MaxService is the largest representable CEA-708 service number.
const MaxService = 63

// WindowCount is the number of windows owned by one service (CW0..CW7).
const WindowCount = 8

// Default canvas geometry used when the caller does not supply one. CEA-708
// window placement is normally proportional; the cell grid here is a
// deterministic, inspectable presentation of the safe-title area.
const (
	DefaultCols = 32
	DefaultRows = 15
)

// Cell is one composited character cell. Owner/Window record where a glyph
// came from, which makes cross-service "ghost text" mechanically detectable.
type Cell struct {
	Rune   rune
	Owner  ServiceID
	Window int
}

func blankCell() Cell {
	return Cell{Rune: ' ', Owner: NoService, Window: -1}
}

// Canvas is a fixed row-major cell grid. Composition always starts from a
// cleared canvas so that stale glyphs can never survive a present.
type Canvas struct {
	Cols  int
	Rows  int
	Cells []Cell
}

// NewCanvas creates a blank Cols x Rows grid.
func NewCanvas(cols, rows int) *Canvas {
	if cols <= 0 {
		cols = DefaultCols
	}
	if rows <= 0 {
		rows = DefaultRows
	}
	c := &Canvas{Cols: cols, Rows: rows, Cells: make([]Cell, cols*rows)}
	c.Clear()
	return c
}

// Clear resets every cell to blank background. This is the primitive behind
// both service-boundary clears and full recomposition.
func (c *Canvas) Clear() {
	for i := range c.Cells {
		c.Cells[i] = blankCell()
	}
}

func (c *Canvas) index(row, col int) int { return row*c.Cols + col }

// InBounds reports whether row/col address a cell.
func (c *Canvas) InBounds(row, col int) bool {
	return row >= 0 && row < c.Rows && col >= 0 && col < c.Cols
}

// Set writes a cell; out-of-bounds writes are ignored (clipping).
func (c *Canvas) Set(row, col int, cell Cell) {
	if !c.InBounds(row, col) {
		return
	}
	c.Cells[c.index(row, col)] = cell
}

// Get reads a cell; out-of-bounds reads return a blank cell.
func (c *Canvas) Get(row, col int) Cell {
	if !c.InBounds(row, col) {
		return blankCell()
	}
	return c.Cells[c.index(row, col)]
}

// Clone returns a deep copy.
func (c *Canvas) Clone() *Canvas {
	dup := *c
	dup.Cells = append([]Cell(nil), c.Cells...)
	return &dup
}

// TextRows renders the canvas as fixed-width strings. Trailing blank columns
// are trimmed for readability; empty rows become "".
func (c *Canvas) TextRows() []string {
	rows := make([]string, c.Rows)
	for r := 0; r < c.Rows; r++ {
		runes := make([]rune, c.Cols)
		last := -1
		for col := 0; col < c.Cols; col++ {
			cell := c.Get(r, col)
			runes[col] = cell.Rune
			if cell.Rune != ' ' && cell.Rune != 0 {
				last = col
			}
		}
		if last >= 0 {
			rows[r] = string(runes[:last+1])
		}
	}
	return rows
}

// OwnerGrid returns the owning service number for every cell (0 = blank).
func (c *Canvas) OwnerGrid() [][]ServiceID {
	out := make([][]ServiceID, c.Rows)
	for r := 0; r < c.Rows; r++ {
		out[r] = make([]ServiceID, c.Cols)
		for col := 0; col < c.Cols; col++ {
			out[r][col] = c.Get(r, col).Owner
		}
	}
	return out
}
