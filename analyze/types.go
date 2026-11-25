// Package analyze audits ASS vector clips against an output frame and the
// subtitle safe area.
package analyze

import (
	"assclip/ass"
	"assclip/layout"
)

// Config selects the target frame and optional protected subject regions.
// Subject regions are in output pixels and can be repeated on the command line.
type Config struct {
	Canvas   layout.Canvas
	Subjects []layout.Rect
}

// Report is the machine-readable analysis result.
type Report struct {
	Script   ScriptReport  `json:"script"`
	Target   layout.Canvas `json:"target"`
	Frame    layout.Rect   `json:"frame"`
	Summary  Summary       `json:"summary"`
	Cues     []CueReport   `json:"cues"`
	Warnings []string      `json:"warnings,omitempty"`
}

type ScriptReport struct {
	PlayResX      float64  `json:"play_res_x"`
	PlayResY      float64  `json:"play_res_y"`
	WrapStyle     int      `json:"wrap_style"`
	ScaledBorders bool     `json:"scaled_border_and_shadow"`
	StyleCount    int      `json:"style_count"`
	CueCount      int      `json:"cue_count"`
	ParseWarnings []string `json:"parse_warnings,omitempty"`
}

type Summary struct {
	Cues           int `json:"cues"`
	CuesWithClip   int `json:"cues_with_clip"`
	CuesWithErrors int `json:"cues_with_errors"`
	Conflicts      int `json:"conflicts"`
	Errors         int `json:"errors"`
	Warnings       int `json:"warnings"`
}

// CueReport contains the converted clip and every detected safety conflict for
// one Dialogue row.
type CueReport struct {
	Index         int             `json:"index"`
	Line          int             `json:"line"`
	Start         string          `json:"start"`
	End           string          `json:"end"`
	Style         string          `json:"style"`
	Alignment     int             `json:"alignment"`
	Text          string          `json:"text"`
	MarginsScript layout.Insets   `json:"margins_script"`
	MarginsPixels layout.Insets   `json:"margins_pixels"`
	SafeArea      layout.SafeArea `json:"safe_area"`
	EstimatedText *layout.Rect    `json:"estimated_text_bounds,omitempty"`
	Clip          *ClipReport     `json:"clip"`
	Conflicts     []Conflict      `json:"conflicts"`
	Warnings      []string        `json:"warnings,omitempty"`
}

// ClipReport is the effective (last) vector clip for a cue, converted to output
// pixels.
type ClipReport struct {
	Mode          ass.ClipMode    `json:"mode"`
	Source        string          `json:"source"`
	IsVector      bool            `json:"is_vector"`
	PointCount    int             `json:"point_count"`
	ScriptBounds  ass.Bounds      `json:"bounds_script"`
	Bounds        layout.Rect     `json:"bounds_pixels"`
	ClampedBounds layout.Rect     `json:"bounds_clamped_to_frame"`
	FrameOverflow layout.Overflow `json:"frame_overflow"`
}

// Conflict describes a concrete crop or safe-area problem.
type Conflict struct {
	Code     string           `json:"code"`
	Severity string           `json:"severity"`
	Message  string           `json:"message"`
	Overflow *layout.Overflow `json:"overflow,omitempty"`
	Area     float64          `json:"overlap_area_px2,omitempty"`
	Zone     string           `json:"zone,omitempty"`
}
