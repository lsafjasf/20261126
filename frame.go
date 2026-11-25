package cea708

import "strings"

// FrameKind labels why a Frame entered the visible timeline.
type FrameKind string

const (
	// FramePresent is a normal post-batch composition of the selected service.
	FramePresent FrameKind = "present"
	// FrameSwitch is the atomic frame emitted by a service selection. It is
	// composited on a cleared canvas and contains only the newly selected
	// service, which is the core anti-overlap guarantee.
	FrameSwitch FrameKind = "switch"
)

// Frame is an immutable presented screen. Every frame belongs to exactly one
// service; composition always starts from a blank canvas, so a frame can never
// contain glyphs inherited from another service.
type Frame struct {
	// Index is the 1-based position in the Manager presentation timeline.
	Index int64 `json:"index"`
	// StreamSeq is the high-water network sequence applied when produced.
	StreamSeq uint64 `json:"stream_seq"`
	// Service is the sole owner of all non-blank cells in Lines.
	Service ServiceID `json:"service"`
	Kind    FrameKind `json:"kind"`
	Width   int       `json:"width"`
	Height  int       `json:"height"`
	// Lines are fixed-height text rows (trailing spaces trimmed per row).
	Lines []string `json:"lines"`
	// OwnerGrid records the owning service per cell (0 = blank). It makes
	// cross-service ghosting mechanically detectable.
	OwnerGrid [][]ServiceID `json:"owner_grid,omitempty"`
}

// Contains reports whether substr appears in any text row.
func (f Frame) Contains(substr string) bool {
	for _, line := range f.Lines {
		if strings.Contains(line, substr) {
			return true
		}
	}
	return false
}

// NonEmptyLines returns rows that contain visible glyphs.
func (f Frame) NonEmptyLines() []string {
	out := make([]string, 0, len(f.Lines))
	for _, line := range f.Lines {
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}
