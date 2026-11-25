package audio

import "fmt"

// ChannelLayout describes a speaker layout by channel count and a speaker
// position mask (LSB ordering: FL, FR, FC, LFE, BL, BR, SL, SR).
type ChannelLayout struct {
	Channels int
	Mask     uint64
}

const (
	SpeakerFL uint64 = 1 << iota
	SpeakerFR
	SpeakerFC
	SpeakerLFE
	SpeakerBL
	SpeakerBR
	SpeakerSL
	SpeakerSR
)

var (
	LayoutMono      = ChannelLayout{Channels: 1, Mask: SpeakerFC}
	LayoutStereo    = ChannelLayout{Channels: 2, Mask: SpeakerFL | SpeakerFR}
	LayoutQuad      = ChannelLayout{Channels: 4, Mask: SpeakerFL | SpeakerFR | SpeakerBL | SpeakerBR}
	LayoutSurround5 = ChannelLayout{Channels: 6, Mask: SpeakerFL | SpeakerFR | SpeakerFC | SpeakerLFE | SpeakerBL | SpeakerBR}
)

func (l ChannelLayout) String() string {
	switch l {
	case LayoutMono:
		return "mono"
	case LayoutStereo:
		return "stereo"
	case LayoutQuad:
		return "quad"
	case LayoutSurround5:
		return "5.1"
	default:
		return fmt.Sprintf("%dch(mask=%#x)", l.Channels, l.Mask)
	}
}

// SampleFormat is the PCM sample representation on the wire.
type SampleFormat int

const (
	FormatS16 SampleFormat = iota
	FormatF32
)

func (f SampleFormat) String() string {
	if f == FormatF32 {
		return "f32"
	}
	return "s16"
}

// StreamParams describes a fully-specified audio stream.
type StreamParams struct {
	SampleRate int
	Layout     ChannelLayout
	Format     SampleFormat
}

func (p StreamParams) String() string {
	return fmt.Sprintf("%d Hz %s %s", p.SampleRate, p.Layout, p.Format)
}

// Capabilities describes what a device can do. An empty slice field means
// "nothing supported" for that dimension.
type Capabilities struct {
	SampleRates []int
	Layouts     []ChannelLayout
	Formats     []SampleFormat
}

func (c Capabilities) supportsRate(rate int) bool {
	for _, r := range c.SampleRates {
		if r == rate {
			return true
		}
	}
	return false
}

func (c Capabilities) supportsLayout(l ChannelLayout) bool {
	for _, cl := range c.Layouts {
		if cl == l {
			return true
		}
	}
	return false
}

func (c Capabilities) supportsFormat(f SampleFormat) bool {
	for _, sf := range c.Formats {
		if sf == f {
			return true
		}
	}
	return false
}
