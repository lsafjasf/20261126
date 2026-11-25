package audio

import (
	"errors"
	"fmt"
)

// ErrNoUsableDevice is returned when the device cannot support any
// reasonable stream configuration at all.
var ErrNoUsableDevice = errors.New("audio: device capabilities cannot carry the stream")

// Fallback describes one dimension where the device could not satisfy the
// desired parameters exactly.
type Fallback struct {
	Field    string
	Desired  string
	Selected string
}

// Negotiation is the outcome of matching desired stream parameters against
// device capabilities.
type Negotiation struct {
	Desired   StreamParams
	Selected  StreamParams
	Fallbacks []Fallback
}

// Exact reports whether the device supports the desired parameters as-is.
func (n Negotiation) Exact() bool { return len(n.Fallbacks) == 0 }

// Negotiate picks the best stream parameters the device can actually drive.
// Exact matches win; otherwise the closest supported value is chosen per
// dimension and recorded as a Fallback (partial support).
func Negotiate(desired StreamParams, caps Capabilities) (Negotiation, error) {
	n := Negotiation{Desired: desired}
	if len(caps.SampleRates) == 0 || len(caps.Layouts) == 0 || len(caps.Formats) == 0 {
		return n, ErrNoUsableDevice
	}

	rate := desired.SampleRate
	if !caps.supportsRate(rate) {
		rate = nearestRate(desired.SampleRate, caps.SampleRates)
		n.Fallbacks = append(n.Fallbacks, Fallback{
			Field:    "sample_rate",
			Desired:  fmt.Sprintf("%d", desired.SampleRate),
			Selected: fmt.Sprintf("%d", rate),
		})
	}

	layout := desired.Layout
	if !caps.supportsLayout(layout) {
		layout = nearestLayout(desired.Layout, caps.Layouts)
		n.Fallbacks = append(n.Fallbacks, Fallback{
			Field:    "channel_layout",
			Desired:  desired.Layout.String(),
			Selected: layout.String(),
		})
	}

	format := desired.Format
	if !caps.supportsFormat(format) {
		format = caps.Formats[0]
		n.Fallbacks = append(n.Fallbacks, Fallback{
			Field:    "sample_format",
			Desired:  desired.Format.String(),
			Selected: format.String(),
		})
	}

	n.Selected = StreamParams{SampleRate: rate, Layout: layout, Format: format}
	return n, nil
}

// nearestRate picks the supported rate closest to want, preferring the
// higher rate on ties (more headroom for downsampling).
func nearestRate(want int, rates []int) int {
	best := rates[0]
	for _, r := range rates[1:] {
		dBest := absDiff(best, want)
		dR := absDiff(r, want)
		if dR < dBest || (dR == dBest && r > best) {
			best = r
		}
	}
	return best
}

// nearestLayout picks the supported layout closest to want: same channel
// count first, then the smallest channel-count difference, preferring
// downmix (fewer channels) over upmix on ties.
func nearestLayout(want ChannelLayout, layouts []ChannelLayout) ChannelLayout {
	best := layouts[0]
	bestScore := layoutScore(want, best)
	for _, l := range layouts[1:] {
		if s := layoutScore(want, l); s < bestScore {
			best, bestScore = l, s
		}
	}
	return best
}

func layoutScore(want, have ChannelLayout) int {
	if have.Channels == want.Channels {
		return 1 // right size, wrong mask
	}
	d := absDiff(have.Channels, want.Channels) * 10
	if have.Channels > want.Channels {
		d += 5 // penalize upmix more than downmix
	}
	return d
}

func absDiff(a, b int) int {
	if a > b {
		return a - b
	}
	return b - a
}
