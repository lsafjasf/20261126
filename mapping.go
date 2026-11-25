package aache

import (
	"math/big"
	"sort"
	"time"
)

// FrameAtOutputSample finds the frame containing a cumulative output sample.
// Sample positions are half-open: [OutputStart, OutputStart+OutputSamples).
func (t *Timeline) FrameAtOutputSample(sample int64) (FrameTiming, bool) {
	if sample < 0 || sample >= t.OutputSamples {
		return FrameTiming{}, false
	}
	index := sort.Search(len(t.Frames), func(index int) bool {
		frame := t.Frames[index]
		return sample < frame.OutputStart+frame.OutputSamples
	})
	if index == len(t.Frames) {
		return FrameTiming{}, false
	}
	return t.Frames[index], true
}

// TimeAtOutputSample converts a cumulative output sample position to
// presentation time. The lookup is frame-aware, so it remains correct when the
// underlying output rate changes.
func (t *Timeline) TimeAtOutputSample(sample int64) (time.Duration, bool) {
	frame, ok := t.FrameAtOutputSample(sample)
	if !ok {
		return 0, false
	}
	offset := sample - frame.OutputStart
	delta := ratDuration(new(big.Rat).SetFrac64(offset, int64(frame.OutputRate)))
	return frame.Start + delta, true
}

// FrameAtTime finds the frame containing a presentation timestamp.
func (t *Timeline) FrameAtTime(at time.Duration) (FrameTiming, bool) {
	if at < 0 || at >= t.Duration {
		return FrameTiming{}, false
	}
	index := sort.Search(len(t.Frames), func(index int) bool {
		return at < t.Frames[index].End
	})
	if index == len(t.Frames) {
		return FrameTiming{}, false
	}
	return t.Frames[index], true
}
