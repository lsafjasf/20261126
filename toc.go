package opusalign

import "fmt"

const (
	SampleRate            = 48000
	MaxPacketSamples      = 5760
	MinPacketSamples      = 120
	PreSkipFieldLength    = 2
	OpusHeadPreSkipOffset = 10
)

type TOC struct {
	Config          int  `json:"config"`
	Stereo          bool `json:"stereo"`
	Code            int  `json:"code"`
	FrameSamples    int  `json:"frame_samples"`
	FrameCount      int  `json:"frame_count"`
	Padding         bool `json:"padding,omitempty"`
	VBR             bool `json:"vbr,omitempty"`
	DurationSamples int  `json:"duration_samples"`
	DurationKnown   bool `json:"duration_known"`
}

func ParseTOCByte(b byte) TOC {
	config := int(b >> 3)
	code := int(b & 0x03)
	toc := TOC{
		Config:       config,
		Stereo:       b&0x04 != 0,
		Code:         code,
		FrameSamples: configFrameSamples(config),
	}

	switch code {
	case 0:
		toc.FrameCount = 1
		toc.DurationKnown = true
	case 1, 2:
		toc.FrameCount = 2
		toc.DurationKnown = true
	case 3:
		toc.FrameCount = 0
	}
	if toc.DurationKnown {
		toc.DurationSamples = toc.FrameSamples * toc.FrameCount
	}
	return toc
}

func PacketDurationSamples(packet []byte) (int, TOC, error) {
	if len(packet) == 0 {
		return 0, TOC{}, fmt.Errorf("opus packet is empty")
	}
	toc := ParseTOCByte(packet[0])
	if toc.Code != 3 {
		if err := validateTOCDuration(toc.DurationSamples); err != nil {
			return 0, toc, err
		}
		return toc.DurationSamples, toc, nil
	}
	if len(packet) < 2 {
		return 0, toc, fmt.Errorf("code 3 packet lacks frame-count byte")
	}

	frameByte := packet[1]
	toc.VBR = frameByte&0x80 != 0
	toc.Padding = frameByte&0x40 != 0
	toc.FrameCount = int(frameByte & 0x3f)
	if toc.FrameCount < 1 || toc.FrameCount > 48 {
		return 0, toc, fmt.Errorf("invalid Opus frame count %d", toc.FrameCount)
	}
	toc.DurationSamples = toc.FrameSamples * toc.FrameCount
	toc.DurationKnown = true
	if err := validateTOCDuration(toc.DurationSamples); err != nil {
		return 0, toc, err
	}
	return toc.DurationSamples, toc, nil
}

func IsLegalDurationSamples(duration int64) bool {
	return duration >= MinPacketSamples &&
		duration <= MaxPacketSamples &&
		duration%MinPacketSamples == 0
}

func configFrameSamples(config int) int {
	switch {
	case config < 12:
		return []int{480, 960, 1920, 2880}[config&3]
	case config < 16:
		return []int{480, 960}[config&1]
	default:
		return []int{120, 240, 480, 960}[(config-16)&3]
	}
}

func validateTOCDuration(duration int) error {
	if !IsLegalDurationSamples(int64(duration)) {
		return fmt.Errorf("illegal Opus packet duration %d samples", duration)
	}
	return nil
}
