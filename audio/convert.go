package audio

// This file contains the render-side fix for the crackle bug: when the
// device renegotiates to new parameters, the player must convert its source
// frames into the negotiated layout/rate instead of pushing the stale
// layout onto the wire. Samples are interleaved float32.

// speakerOrder returns the speaker bits of a mask in canonical LSB order.
func speakerOrder(mask uint64) []uint64 {
	var out []uint64
	for b := uint(0); b < 64; b++ {
		if mask&(1<<b) != 0 {
			out = append(out, 1<<b)
		}
	}
	return out
}

// mixCoeff returns the gain applied when mapping source speaker src into
// destination speaker dst. Surrounds fold into their own side only.
func mixCoeff(src, dst uint64) float32 {
	if src == dst {
		return 1
	}
	switch dst {
	case SpeakerFL:
		if src == SpeakerFC || src == SpeakerBL || src == SpeakerSL {
			return 0.707
		}
	case SpeakerFR:
		if src == SpeakerFC || src == SpeakerBR || src == SpeakerSR {
			return 0.707
		}
	case SpeakerFC:
		// Derive a phantom center from the fronts.
		if src == SpeakerFL || src == SpeakerFR {
			return 0.5
		}
	}
	return 0
}

// RemapLayout converts interleaved frames between channel layouts using
// standard downmix coefficients; missing destination speakers are derived
// where possible, otherwise left silent.
func RemapLayout(src []float32, from, to ChannelLayout) []float32 {
	if from == to {
		out := make([]float32, len(src))
		copy(out, src)
		return out
	}
	if from.Channels == 1 {
		// Mono duplicates to every destination channel at unity gain.
		frames := len(src)
		dst := make([]float32, frames*to.Channels)
		for f := 0; f < frames; f++ {
			for c := 0; c < to.Channels; c++ {
				dst[f*to.Channels+c] = src[f]
			}
		}
		return dst
	}
	frames := len(src) / from.Channels
	dst := make([]float32, frames*to.Channels)
	srcSpk := speakerOrder(from.Mask)
	dstSpk := speakerOrder(to.Mask)
	for f := 0; f < frames; f++ {
		for di, ds := range dstSpk {
			var acc float32
			for si, ss := range srcSpk {
				if g := mixCoeff(ss, ds); g != 0 {
					acc += g * src[f*from.Channels+si]
				}
			}
			dst[f*to.Channels+di] = acc
		}
	}
	return dst
}

// ResampleLinear converts interleaved frames between sample rates with
// linear interpolation. Production code would use a polyphase filter; the
// key property here is that the frame count follows the exact rate ratio so
// the Position tracker stays in sync.
func ResampleLinear(src []float32, channels, fromRate, toRate int) []float32 {
	if fromRate == toRate || len(src) == 0 {
		out := make([]float32, len(src))
		copy(out, src)
		return out
	}
	inFrames := len(src) / channels
	outFrames := int(int64(inFrames) * int64(toRate) / int64(fromRate))
	if outFrames == 0 {
		return nil
	}
	dst := make([]float32, outFrames*channels)
	for of := 0; of < outFrames; of++ {
		// Map output frame to fractional input position.
		pos := int64(of) * int64(fromRate)
		i0 := int(pos / int64(toRate))
		frac := float32(pos%int64(toRate)) / float32(toRate)
		i1 := i0 + 1
		if i1 >= inFrames {
			i1 = inFrames - 1
		}
		for c := 0; c < channels; c++ {
			a := src[i0*channels+c]
			b := src[i1*channels+c]
			dst[of*channels+c] = a + frac*(b-a)
		}
	}
	return dst
}

// Convert transforms interleaved f32 frames from the source stream params
// into the negotiated device params: layout first, then rate.
func Convert(src []float32, from, to StreamParams) []float32 {
	out := RemapLayout(src, from.Layout, to.Layout)
	return ResampleLinear(out, to.Layout.Channels, from.SampleRate, to.SampleRate)
}
