package subtitle

import "time"

// Format identifies the subtitle container.
type Format string

const (
	FormatSRT Format = "srt"
	FormatVTT Format = "vtt"
	FormatASS Format = "ass"
)

// FontRef records a font selected by the subtitle itself. It is evidence
// only: the audit never silently swaps fonts, it reports which font name the
// render pipeline was asked to use and where the request came from.
type FontRef struct {
	Name   string `json:"name"`
	Source string `json:"source"`
}

// Cue is one timed text entry. Visible is the markup-free text that actually
// reaches the renderer; RawText keeps the original dialogue body.
type Cue struct {
	Index   int           `json:"index"`
	Start   time.Duration `json:"start"`
	End     time.Duration `json:"end"`
	Style   string        `json:"style,omitempty"`
	Visible string        `json:"visible"`
	RawText string        `json:"rawText"`
	Fonts   []FontRef     `json:"fonts,omitempty"`
}

// File is the decoded subtitle document.
type File struct {
	Path     string    `json:"path"`
	Format   Format    `json:"format"`
	Cues     []Cue     `json:"cues"`
	Fonts    []FontRef `json:"fonts,omitempty"`
	Warnings []string  `json:"warnings,omitempty"`
}

// Timestamp prints a duration as HH:MM:SS.mmm, the common subtitle notation.
func Timestamp(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	ms := d.Milliseconds()
	h := ms / 3_600_000
	ms %= 3_600_000
	m := ms / 60_000
	ms %= 60_000
	s := ms / 1_000
	ms %= 1_000
	return format2(h) + ":" + format2(m) + ":" + format2(s) + "." + format3(ms)
}

func format2(v int64) string {
	if v < 10 {
		return "0" + itoa(v)
	}
	return itoa(v)
}

func format3(v int64) string {
	switch {
	case v < 10:
		return "00" + itoa(v)
	case v < 100:
		return "0" + itoa(v)
	default:
		return itoa(v)
	}
}

func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
