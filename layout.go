package cuemap

import (
	"errors"
	"fmt"
	"unicode/utf8"
)

// FallbackReason is a stable machine-readable explanation for returning the
// caller to the original cue.
type FallbackReason string

const (
	FallbackInvalidRuby   FallbackReason = "invalid_ruby"
	FallbackEmptyRuby     FallbackReason = "empty_ruby"
	FallbackOverlapRuby   FallbackReason = "overlapping_ruby"
	FallbackCrossLineRuby FallbackReason = "ruby_crosses_line"
	FallbackBounds        FallbackReason = "layout_bounds_exceeded"
	FallbackCollision     FallbackReason = "glyph_collision"
	FallbackUnsupported   FallbackReason = "unsupported_layout"
)

// Fallback points back to the source cue when deterministic layout is unsafe.
type Fallback struct {
	Reason       FallbackReason `json:"reason"`
	Message      string         `json:"message"`
	CueID        string         `json:"cue_id"`
	CueRef       string         `json:"cue_ref"`
	OriginalText string         `json:"original_text"`
	Cue          Cue            `json:"cue"`
}

// Options controls deterministic layout. The zero value uses DefaultMetrics.
type Options struct {
	Direction Direction   `json:"direction,omitempty"`
	Mode      WritingMode `json:"mode,omitempty"`
	Metrics   Metrics     `json:"metrics,omitempty"`
}

// Metrics are integer pixel-like units. Using integers keeps tests and fixture
// generation deterministic without font files or platform text shaping.
type Metrics struct {
	BaseSize    int `json:"base_size"`
	RubySize    int `json:"ruby_size"`
	RubyGap     int `json:"ruby_gap"`
	CharSpacing int `json:"char_spacing"`
	LineSpacing int `json:"line_spacing"`
	Padding     int `json:"padding"`
	MaxWidth    int `json:"max_width,omitempty"`
	MaxHeight   int `json:"max_height,omitempty"`
}

// DefaultMetrics returns conservative subtitle-sized metrics.
func DefaultMetrics() Metrics {
	return Metrics{
		BaseSize:    24,
		RubySize:    12,
		RubyGap:     4,
		CharSpacing: 0,
		LineSpacing: 10,
		Padding:     8,
	}
}

func (m Metrics) normalized() Metrics {
	d := DefaultMetrics()
	if m.BaseSize <= 0 {
		m.BaseSize = d.BaseSize
	}
	if m.RubySize <= 0 {
		m.RubySize = m.BaseSize / 2
	}
	if m.RubyGap < 0 {
		m.RubyGap = 0
	} else if m.RubyGap == 0 {
		m.RubyGap = d.RubyGap
	}
	if m.CharSpacing < 0 {
		m.CharSpacing = 0
	}
	if m.LineSpacing < 0 {
		m.LineSpacing = 0
	} else if m.LineSpacing == 0 {
		m.LineSpacing = d.LineSpacing
	}
	if m.Padding < 0 {
		m.Padding = 0
	} else if m.Padding == 0 {
		m.Padding = d.Padding
	}
	return m
}

// Result is either a collision-free layout or a reference to the original cue.
type Result struct {
	CueID    string       `json:"cue_id"`
	OK       bool         `json:"ok"`
	Mode     WritingMode  `json:"mode"`
	Bounds   Rect         `json:"bounds"`
	Lines    []LineLayout `json:"lines,omitempty"`
	Fallback *Fallback    `json:"fallback,omitempty"`
}

// CellKind distinguishes base glyph cells from ruby annotation cells.
type CellKind string

const (
	CellBase CellKind = "base"
	CellRuby CellKind = "ruby"
)

// Cell is one positioned glyph run/box. Text remains logical within a cluster.
type Cell struct {
	Kind           CellKind `json:"kind"`
	Text           string   `json:"text"`
	ScreenSlot     int      `json:"screen_slot,omitempty"`
	LogicalCluster int      `json:"logical_cluster,omitempty"`
	RubyIndex      int      `json:"ruby_index,omitempty"`
	Rect           Rect     `json:"rect"`
}

// LineLayout contains mapping and positioned cells for one hard cue line.
type LineLayout struct {
	Index       int     `json:"index"`
	Mapping     Mapping `json:"mapping"`
	Cells       []Cell  `json:"cells"`
	BaseCells   []Cell  `json:"base_cells"`
	RubyCells   []Cell  `json:"ruby_cells"`
	Bounds      Rect    `json:"bounds"`
	BasePitch   int     `json:"base_pitch"`
	ColumnPitch int     `json:"column_pitch,omitempty"`
}

// Typeset builds the mapping and collision-free geometry for a cue.
//
// Layout limitations are returned as Result.Fallback, not as a Go error, so a
// renderer can directly use Fallback.Cue/Raw. Validation failures that indicate
// malformed input also produce a fallback result.
func Typeset(cue Cue, opts Options) Result {
	metrics := opts.Metrics.normalized()
	mode := opts.Mode
	if mode == ModeUnspecified {
		mode = cue.Mode
	}
	if mode == ModeUnspecified {
		mode = ModeHorizontal
	}
	direction := opts.Direction
	if direction == DirectionAuto {
		direction = cue.Direction
	}

	if err := ValidateRuby(cue); err != nil {
		var layoutErr *LayoutError
		reason := FallbackInvalidRuby
		if errors.As(err, &layoutErr) {
			reason = layoutErr.Code
		}
		return fallbackResult(cue, reason, err.Error())
	}
	if reason, message := validateLineBoundaries(cue); reason != "" {
		return fallbackResult(cue, reason, message)
	}

	lineTexts := splitCueLines(cue.Text)
	lineOffsets := make([]int, len(lineTexts))
	offset := 0
	for i, line := range lineTexts {
		lineOffsets[i] = offset
		offset += utf8.RuneCountInString(line) + 1
	}

	layout := Result{CueID: cue.ID, OK: true, Mode: mode, Lines: make([]LineLayout, 0, len(lineTexts))}
	for lineIndex, lineText := range lineTexts {
		mapping := MapLine(lineText, direction)
		localRuby := rubyForLine(cue.Ruby, lineIndex, lineOffsets, lineText)

		var line LineLayout
		switch mode {
		case ModeVertical:
			line = placeVerticalLine(lineIndex, mapping, localRuby, metrics)
		default:
			line = placeHorizontalLine(lineIndex, mapping, localRuby, metrics)
		}
		layout.Lines = append(layout.Lines, line)
	}

	layout.Bounds = stackLineBounds(layout.Lines, mode, metrics)
	if reason := checkResultBounds(layout.Bounds, metrics); reason != "" {
		return fallbackResult(cue, reason, fmt.Sprintf("layout bounds %+v exceed metrics limits", layout.Bounds))
	}
	if collision := FindCollision(layout); collision != nil {
		return fallbackResult(cue, FallbackCollision, "positioned cells overlap: "+collision.Error())
	}
	return layout
}

func fallbackResult(cue Cue, reason FallbackReason, message string) Result {
	original := cue.Raw
	if original == "" {
		original = cue.Text
	}
	ref := cue.ID
	if ref != "" {
		ref = "cue:" + cue.ID
	}
	return Result{
		CueID: cue.ID,
		OK:    false,
		Fallback: &Fallback{
			Reason:       reason,
			Message:      message,
			CueID:        cue.ID,
			CueRef:       ref,
			OriginalText: original,
			Cue:          cue,
		},
	}
}

func validateLineBoundaries(cue Cue) (FallbackReason, string) {
	runeIndex := 0
	for _, r := range cue.Text {
		if r == '\n' {
			for _, span := range cue.Ruby {
				if span.StartRune < runeIndex && runeIndex < span.EndRune {
					return FallbackCrossLineRuby, "ruby span crosses a cue line break"
				}
			}
		}
		runeIndex++
	}
	return "", ""
}

func rubyForLine(spans []RubySpan, lineIndex int, offsets []int, line string) []RubySpan {
	startGlobal := offsets[lineIndex]
	endGlobal := startGlobal + utf8.RuneCountInString(line)
	out := make([]RubySpan, 0)
	for _, span := range spans {
		if span.StartRune >= startGlobal && span.EndRune <= endGlobal {
			local := span
			local.StartRune -= startGlobal
			local.EndRune -= startGlobal
			out = append(out, local)
		}
	}
	return out
}

func checkResultBounds(bounds Rect, metrics Metrics) FallbackReason {
	if metrics.MaxWidth > 0 && bounds.Width > metrics.MaxWidth {
		return FallbackBounds
	}
	if metrics.MaxHeight > 0 && bounds.Height > metrics.MaxHeight {
		return FallbackBounds
	}
	return ""
}
