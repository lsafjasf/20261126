package cuemap

// Mapping is the logical-to-screen mapping for one hard line.
type Mapping struct {
	// Logical is the original logical-order line.
	Logical string `json:"logical"`
	// Visual is a diagnostic concatenation in screen left-to-right order.
	// Bidi-aware renderers should still receive Logical; use ScreenOrder and
	// cell positions for custom rendering.
	Visual string `json:"visual"`

	BaseDirection Direction `json:"base_direction"`
	BaseLevel     int       `json:"base_level"`
	Levels        []int     `json:"levels"`

	Clusters []Cluster  `json:"clusters"`
	Runes    []RuneInfo `json:"runes"`

	// ScreenOrder[screenSlot] = logical cluster index.
	ScreenOrder []int `json:"screen_order"`
	// LogicalToScreen[logicalCluster] = screen slot.
	LogicalToScreen []int `json:"logical_to_screen"`
	// RuneToScreen[logicalRune] = screen slot. Combining marks share a slot
	// with their base cluster.
	RuneToScreen []int `json:"rune_to_screen"`
	// ScreenToRunes[screenSlot] = logical rune indexes in that glyph cluster.
	ScreenToRunes [][]int `json:"screen_to_runes"`
}

// WritingMode selects the block/inline geometry used by layout.
type WritingMode int

const (
	// ModeUnspecified defaults to Horizontal unless options say otherwise.
	ModeUnspecified WritingMode = iota
	// ModeHorizontal lays text left-to-right in screen slots and stacks
	// lines downward. Bidi runs are reordered within each line.
	ModeHorizontal
	// ModeVertical places screen slots top-to-bottom and stacks columns
	// from right to left.
	ModeVertical
)

// Cue is one subtitle cue. Text is plain logical text; Ruby offsets are rune
// indexes into Text. Raw should retain the source cue/markup for fallback.
type Cue struct {
	ID        string      `json:"id"`
	Index     int         `json:"index,omitempty"`
	StartMS   int64       `json:"start_ms,omitempty"`
	EndMS     int64       `json:"end_ms,omitempty"`
	Language  string      `json:"language,omitempty"`
	Text      string      `json:"text"`
	Raw       string      `json:"raw,omitempty"`
	Direction Direction   `json:"direction,omitempty"`
	Mode      WritingMode `json:"mode,omitempty"`
	Ruby      []RubySpan  `json:"ruby,omitempty"`
}

// RubySpan attaches phonetic text to the rune interval [StartRune,EndRune).
type RubySpan struct {
	StartRune int    `json:"start_rune"`
	EndRune   int    `json:"end_rune"`
	Base      string `json:"base,omitempty"`
	Text      string `json:"text"`
}

// Rect is an integer rectangle in deterministic milli-layout units? Layout uses
// pixel-like units supplied by Metrics; zero-value coordinates are valid.
type Rect struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// Right returns the exclusive right edge.
func (r Rect) Right() int { return r.X + r.Width }

// Bottom returns the exclusive bottom edge.
func (r Rect) Bottom() int { return r.Y + r.Height }

// Intersects reports whether two rectangles overlap with positive area.
func (r Rect) Intersects(other Rect) bool {
	return r.X < other.Right() && r.Right() > other.X &&
		r.Y < other.Bottom() && r.Bottom() > other.Y
}
