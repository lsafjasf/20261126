// Package ass parses the parts of an ASS/SSA subtitle file needed to audit
// vector clips and subtitle layout.
package ass

import (
	"strings"
	"time"
)

// ScriptInfo contains the global canvas information from [Script Info].
type ScriptInfo struct {
	PlayResX              float64
	PlayResY              float64
	PlayResXSet           bool
	PlayResYSet           bool
	ScaledBorderAndShadow bool
	WrapStyle             int
}

// Style is one row from [V4+ Styles] or [V4 Styles].
type Style struct {
	Name      string
	FontName  string
	FontSize  float64
	ScaleX    float64
	ScaleY    float64
	Spacing   float64
	Alignment int
	MarginL   float64
	MarginR   float64
	MarginV   float64
	Outline   float64
	Shadow    float64
	Line      int
}

// Event is one Dialogue row. Margin values of zero mean "use the style
// margin", as defined by ASS.
type Event struct {
	Layer   int
	Start   time.Duration
	End     time.Duration
	Style   string
	Name    string
	MarginL float64
	MarginR float64
	MarginV float64
	Effect  string
	Text    string
	Line    int
}

// Script is the parsed subtitle document.
type Script struct {
	Info      ScriptInfo
	Styles    map[string]Style
	StyleList []Style
	Events    []Event
	Warnings  []string
}

// StyleByName returns a style using ASS's case-insensitive name matching.
func (s *Script) StyleByName(name string) (Style, bool) {
	style, ok := s.Styles[strings.ToLower(name)]
	return style, ok
}

// ClipMode distinguishes a normal clip from an inverse clip.
type ClipMode string

const (
	ClipNormal  ClipMode = "clip"
	ClipInverse ClipMode = "iclip"
)

// Point is a point in ASS script coordinates.
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Bounds is an axis-aligned rectangle in script coordinates.
type Bounds struct {
	MinX float64 `json:"min_x"`
	MinY float64 `json:"min_y"`
	MaxX float64 `json:"max_x"`
	MaxY float64 `json:"max_y"`
}

// Clip is one parsed \clip or \iclip override.
type Clip struct {
	Mode     ClipMode `json:"mode"`
	Source   string   `json:"source"`
	IsVector bool     `json:"is_vector"`
	Points   []Point  `json:"points,omitempty"`
	Bounds   Bounds   `json:"bounds"`
}

// Override is the subset of ASS override tags used by layout auditing.
type Override struct {
	HasAlignment bool
	Alignment    int
	FontSize     *float64
	ScaleX       *float64
	ScaleY       *float64
	Outline      *float64
	Shadow       *float64
	Position     *Point
	MoveFrom     *Point
	MoveTo       *Point
	Clips        []Clip
	VisibleText  string
}
