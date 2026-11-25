// Package vttlayout detects and repairs WebVTT region occlusion that appears
// when a presentation switches from a landscape to a portrait canvas.
//
// The pipeline is:
//
//	Parse -> Geometry (authored boxes on the target canvas) -> Timeline
//	(cluster cues by overlap) -> Solve (priority aware packing using the
//	wrapped line count and the available safe canvas) -> Repair candidates
//	(time extension / sequential cue splits) -> Render preview frames.
//
// Every check and every preview frame is produced by Go code with no external
// binaries.
package vttlayout

import (
	"fmt"
	"time"
)

// Milliseconds is the integer time unit used across the whole package.
type Milliseconds = int64

// Region is a parsed WebVTT REGION block.
//
// Percentages are stored as values in [0,100]. Priority is an extension
// setting ("priority:N") accepted on both regions and cues; larger values
// mean higher priority.
type Region struct {
	ID              string
	WidthPct        float64
	Lines           int
	RegionAnchorX   float64
	RegionAnchorY   float64
	ViewportAnchorX float64
	ViewportAnchorY float64
	Scroll          string
	Priority        int
	PrioritySet     bool
}

// CueSettings is the parsed cue setting list ("region:.. line:.. ...").
type CueSettings struct {
	Region        string
	HasLinePct    bool
	LinePct       float64
	HasLineNumber bool
	LineNumber    float64
	LineAlign     string
	SnapToLines   bool
	HasPosition   bool
	PositionPct   float64
	PositionAlign string
	HasSize       bool
	SizePct       float64
	Align         string
	Vertical      string
	Priority      int
	PrioritySet   bool
}

// Cue is one WebVTT cue.
type Cue struct {
	ID          string
	StartMS     Milliseconds
	EndMS       Milliseconds
	RawSettings string
	Settings    CueSettings
	// Text is the plain payload with inline tags removed; explicit payload
	// newlines are preserved.
	Text  string
	Order int
}

// Duration returns the cue duration in milliseconds.
func (c *Cue) Duration() Milliseconds { return c.EndMS - c.StartMS }

// ParseWarning describes a non fatal parser issue.
type ParseWarning struct {
	Line int
	Msg  string
}

// Document is a parsed WebVTT file.
type Document struct {
	Regions     map[string]*Region
	RegionOrder []string
	Cues        []*Cue
	Warnings    []ParseWarning
}

// Priority returns the effective priority for a cue: cue setting wins over
// its region setting, and zero is the default priority.
func (d *Document) Priority(c *Cue) int {
	if c.Settings.PrioritySet {
		return c.Settings.Priority
	}
	if c.Settings.Region != "" {
		if r, ok := d.Regions[c.Settings.Region]; ok && r.PrioritySet {
			return r.Priority
		}
	}
	return 0
}

// ParseTimestamp parses WebVTT timestamps of the form
// "HH:MM:SS.mmm", "MM:SS.mmm" (comma decimal accepted as seen in SRT).
func ParseTimestamp(s string) (Milliseconds, error) {
	var h, m, sec, frac int
	var fracDigits string
	// Split off fractional part.
	main := s
	for i := 0; i < len(s); i++ {
		if s[i] == '.' || s[i] == ',' {
			main = s[:i]
			fracDigits = s[i+1:]
			break
		}
	}
	parts := splitColons(main)
	var err error
	switch len(parts) {
	case 3:
		if h, err = parseInt(parts[0]); err != nil {
			return 0, fmt.Errorf("bad hours in %q: %w", s, err)
		}
		if m, err = parseInt(parts[1]); err != nil {
			return 0, fmt.Errorf("bad minutes in %q: %w", s, err)
		}
		if sec, err = parseInt(parts[2]); err != nil {
			return 0, fmt.Errorf("bad seconds in %q: %w", s, err)
		}
	case 2:
		if m, err = parseInt(parts[0]); err != nil {
			return 0, fmt.Errorf("bad minutes in %q: %w", s, err)
		}
		if sec, err = parseInt(parts[1]); err != nil {
			return 0, fmt.Errorf("bad seconds in %q: %w", s, err)
		}
	default:
		return 0, fmt.Errorf("invalid timestamp %q", s)
	}
	if len(fracDigits) > 0 {
		if len(fracDigits) > 3 {
			fracDigits = fracDigits[:3]
		}
		for len(fracDigits) < 3 {
			fracDigits += "0"
		}
		if frac, err = parseInt(fracDigits); err != nil {
			return 0, fmt.Errorf("bad fraction in %q: %w", s, err)
		}
	}
	if m >= 60 || sec >= 60 {
		return 0, fmt.Errorf("timestamp out of range %q", s)
	}
	return Milliseconds(h)*3600_000 + Milliseconds(m)*60_000 +
		Milliseconds(sec)*1000 + Milliseconds(frac), nil
}

// FormatTimestamp renders ms as "HH:MM:SS.mmm".
func FormatTimestamp(ms Milliseconds) string {
	if ms < 0 {
		ms = 0
	}
	d := time.Duration(ms) * time.Millisecond
	h := int(d / time.Hour)
	m := int(d/time.Minute) % 60
	s := int(d/time.Second) % 60
	f := ms % 1000
	return fmt.Sprintf("%02d:%02d:%02d.%03d", h, m, s, f)
}

// FormatTimestampShort renders ms as "MM:SS.mmm" (hours folded into minutes
// for frame file names).
func FormatTimestampShort(ms Milliseconds) string {
	totalMin := ms / 60_000
	s := (ms / 1000) % 60
	f := ms % 1000
	return fmt.Sprintf("%02d%02d_%03d", totalMin, s, f)
}

func splitColons(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ':' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}

func parseInt(s string) (int, error) {
	if s == "" {
		return 0, fmt.Errorf("empty number")
	}
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("not a number %q", s)
		}
		n = n*10 + int(r-'0')
	}
	return n, nil
}
