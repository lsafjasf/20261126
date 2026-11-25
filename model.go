// Package safereflow re-flows subtitles against time-varying safe areas.
//
// A SafeTrack describes the dynamic safe rectangle as a list of time
// intervals (camera moves make the rectangle change). Cues are intersected
// with every interval overlapping their display time, and the engine proposes
// line-wrapping (拆行), shortening (缩短) and migration (迁移) fixes.
//
// Confidence is a first-class input: a migration is never authorized on
// low-confidence or partially-covered tracking data.
package safereflow

// TimeRange is a half-open time interval [Start, End) in seconds.
type TimeRange struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

// Duration returns End - Start.
func (t TimeRange) Duration() float64 { return t.End - t.Start }

// Contains reports whether ts lies in the half-open interval.
func (t TimeRange) Contains(ts float64) bool {
	return ts >= t.Start && ts < t.End
}

// Overlaps reports whether two intervals share positive time.
func (t TimeRange) Overlaps(o TimeRange) bool {
	return t.Start < o.End-eps && o.Start < t.End-eps
}

// Intersection returns the shared time interval.
func (t TimeRange) Intersection(o TimeRange) (TimeRange, bool) {
	start := maxFloat(t.Start, o.Start)
	end := minFloat(t.End, o.End)
	if end-start <= eps {
		return TimeRange{}, false
	}
	return TimeRange{Start: start, End: end}, true
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// Canvas is the video raster size in pixels.
type Canvas struct {
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// Alignment is a 9-point anchor. Resizing a cue keeps its anchor point fixed;
// migration moves that anchor/box into the safe rectangle.
type Alignment string

const (
	AlignTopLeft      Alignment = "top-left"
	AlignTopCenter    Alignment = "top-center"
	AlignTopRight     Alignment = "top-right"
	AlignMiddleLeft   Alignment = "middle-left"
	AlignMiddleCenter Alignment = "middle-center"
	AlignMiddleRight  Alignment = "middle-right"
	AlignBottomLeft   Alignment = "bottom-left"
	AlignBottomCenter Alignment = "bottom-center"
	AlignBottomRight  Alignment = "bottom-right"
)

// Valid reports whether a is one of the supported alignments.
func (a Alignment) Valid() bool {
	switch a {
	case AlignTopLeft, AlignTopCenter, AlignTopRight,
		AlignMiddleLeft, AlignMiddleCenter, AlignMiddleRight,
		AlignBottomLeft, AlignBottomCenter, AlignBottomRight, "":
		return true
	}
	return false
}

// Style controls deterministic text measurement and cue placement.
//
// All sizes are in canvas pixels. Zero values are replaced by DefaultStyle.
type Style struct {
	FontSize   float64   `json:"font_size,omitempty"`
	LineHeight float64   `json:"line_height,omitempty"` // multiplier of FontSize
	PaddingX   float64   `json:"padding_x,omitempty"`
	PaddingY   float64   `json:"padding_y,omitempty"`
	MaxWidth   float64   `json:"max_width,omitempty"` // wrap width for fresh layout
	MaxLines   int       `json:"max_lines,omitempty"`
	Alignment  Alignment `json:"alignment,omitempty"`
	MarginX    float64   `json:"margin_x,omitempty"`
	MarginY    float64   `json:"margin_y,omitempty"`
}

// DefaultStyle is used when the document or cue does not specify a value.
func DefaultStyle() Style {
	return Style{
		FontSize:   42,
		LineHeight: 1.35,
		PaddingX:   12,
		PaddingY:   8,
		MaxLines:   3,
		Alignment:  AlignBottomCenter,
		MarginX:    96,
		MarginY:    72,
	}
}

// Normalize fills zero fields with defaults and clamps invalid values.
func (s Style) Normalize() Style {
	d := DefaultStyle()
	if s.FontSize <= 0 {
		s.FontSize = d.FontSize
	}
	if s.LineHeight <= 0 {
		s.LineHeight = d.LineHeight
	}
	if s.PaddingX < 0 {
		s.PaddingX = 0
	}
	if s.PaddingY < 0 {
		s.PaddingY = 0
	}
	if s.MaxLines <= 0 {
		s.MaxLines = d.MaxLines
	}
	if !s.Alignment.Valid() || s.Alignment == "" {
		s.Alignment = d.Alignment
	}
	if s.MarginX < 0 {
		s.MarginX = 0
	}
	if s.MarginY < 0 {
		s.MarginY = 0
	}
	return s
}

// MergeOver applies non-zero fields of over on top of s.
func (s Style) MergeOver(over Style) Style {
	if over.FontSize > 0 {
		s.FontSize = over.FontSize
	}
	if over.LineHeight > 0 {
		s.LineHeight = over.LineHeight
	}
	if over.PaddingX > 0 {
		s.PaddingX = over.PaddingX
	}
	if over.PaddingY > 0 {
		s.PaddingY = over.PaddingY
	}
	if over.MaxWidth > 0 {
		s.MaxWidth = over.MaxWidth
	}
	if over.MaxLines > 0 {
		s.MaxLines = over.MaxLines
	}
	if over.Alignment != "" {
		s.Alignment = over.Alignment
	}
	if over.MarginX > 0 {
		s.MarginX = over.MarginX
	}
	if over.MarginY > 0 {
		s.MarginY = over.MarginY
	}
	return s
}

// Cue is one subtitle event. Box is optional: when nil it is derived from
// Text, Style and the canvas via the configured Measurer.
type Cue struct {
	ID string `json:"id"`
	TimeRange
	Text      string `json:"text"`
	Box       *Rect  `json:"box,omitempty"`
	Alignment string `json:"alignment,omitempty"`
	Style     *Style `json:"style,omitempty"`
}

// CueDocument is the JSON input/output for cues.
type CueDocument struct {
	Format string `json:"format,omitempty"`
	Canvas Canvas `json:"canvas"`
	Style  Style  `json:"style,omitempty"`
	Cues   []*Cue `json:"cues"`
}

// EffectiveStyle resolves document defaults + cue override.
func (d *CueDocument) EffectiveStyle(c *Cue) Style {
	s := d.Style.Normalize()
	if c != nil && c.Style != nil {
		s = s.MergeOver(*c.Style)
	}
	if c != nil && c.Alignment != "" {
		s.Alignment = Alignment(c.Alignment)
	}
	return s.Normalize()
}

// SafeSpan is one time interval with its own safe rectangle and confidence.
// Confidence is a pointer so "missing" is distinguishable from an explicit 0.
type SafeSpan struct {
	TimeRange
	ID         string   `json:"id,omitempty"`
	Rect       Rect     `json:"rect"`
	Confidence *float64 `json:"confidence,omitempty"`
	Source     string   `json:"source,omitempty"`
}

// Conf returns the confidence or 0 when unknown.
func (s SafeSpan) Conf() float64 {
	if s.Confidence == nil {
		return 0
	}
	return *s.Confidence
}

// SafeDocument is the on-disk dynamic safe-area track.
//
// Unit may be "pixel" (default) or "normalized": normalized rectangles are
// multiplied by Canvas width/height when the track is prepared.
type SafeDocument struct {
	Canvas              Canvas     `json:"canvas"`
	Unit                string     `json:"unit,omitempty"`
	ConfidenceThreshold *float64   `json:"confidence_threshold,omitempty"`
	SafeAreas           []SafeSpan `json:"safe_areas"`
}
