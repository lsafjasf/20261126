package main

import "fmt"

// Segment is one run of DSD audio at a constant sample rate. A file
// spliced from DSD64 and DSD128 parts is modeled as two segments so the
// time axis follows the true rate of each part instead of one flat rate.
type Segment struct {
	Samples uint64 // per-channel DSD samples in this segment
	Rate    uint32 // DSD sample rate in Hz (2822400 = DSD64, 5644800 = DSD128)
}

// DurationNanos returns the play time of the segment.
func (s Segment) DurationNanos() uint64 {
	if s.Rate == 0 {
		return 0
	}
	return s.Samples * 1e9 / uint64(s.Rate)
}

// boundary is one precomputed segment edge on the time axis.
type boundary struct {
	startSample uint64 // first sample index of the segment
	startNanos  uint64 // timeline position of startSample
	seg         Segment
}

// Timeline maps between sample indices and playback time across segments
// whose DSD rates differ.
type Timeline struct {
	segs   []boundary
	total  uint64 // total samples
	totalN uint64 // total nanoseconds
}

// NewTimeline builds the cumulative axis from the given segments.
func NewTimeline(segs []Segment) (*Timeline, error) {
	t := &Timeline{}
	var sampleBase, nanoBase uint64
	for i, s := range segs {
		if s.Rate == 0 {
			return nil, fmt.Errorf("segment %d: zero sample rate", i)
		}
		t.segs = append(t.segs, boundary{startSample: sampleBase, startNanos: nanoBase, seg: s})
		sampleBase += s.Samples
		nanoBase += s.DurationNanos()
	}
	t.total, t.totalN = sampleBase, nanoBase
	return t, nil
}

// TotalSamples and TotalNanos report the full extent of the axis.
func (t *Timeline) Totals() (samples, nanos uint64) { return t.total, t.totalN }

// TimeAt maps an absolute per-channel sample index to playback time in
// nanoseconds, using the rate of the segment that owns the sample.
func (t *Timeline) TimeAt(sample uint64) uint64 {
	if len(t.segs) == 0 {
		return 0
	}
	b := t.segs[0]
	for _, cand := range t.segs {
		if cand.startSample > sample {
			break
		}
		b = cand
	}
	off := sample - b.startSample
	if off > b.seg.Samples {
		off = b.seg.Samples
	}
	return b.startNanos + off*1e9/uint64(b.seg.Rate)
}

// SampleAt maps playback time in nanoseconds back to a sample index.
func (t *Timeline) SampleAt(nanos uint64) uint64 {
	if len(t.segs) == 0 {
		return 0
	}
	b := t.segs[0]
	for _, cand := range t.segs {
		if cand.startNanos > nanos {
			break
		}
		b = cand
	}
	off := nanos - b.startNanos
	if d := b.seg.DurationNanos(); off > d {
		off = d
	}
	return b.startSample + off*uint64(b.seg.Rate)/1e9
}

// NaiveNanos shows what a PCM-style editor computes: it accumulates every
// DSD sample against one wrong rate (typically a PCM rate like 176400 Hz,
// or the first segment's rate), stretching the axis. The ratio
// NaiveNanos/actual quantifies the timeline blow-up.
func NaiveNanos(totalSamples uint64, wrongRate uint32) uint64 {
	if wrongRate == 0 {
		return 0
	}
	return totalSamples * 1e9 / uint64(wrongRate)
}

// Chapter is a named point on the timeline, stored in ID3 CHAP frames.
type Chapter struct {
	Title  string
	Sample uint64 // absolute per-channel sample index where it starts
}

// ChapterTime resolves a chapter to its start/end nanoseconds using the
// segment-aware axis; the last chapter ends at the timeline total.
func (t *Timeline) ChapterTime(chapters []Chapter, idx int) (start, end uint64) {
	start = t.TimeAt(chapters[idx].Sample)
	if idx+1 < len(chapters) {
		end = t.TimeAt(chapters[idx+1].Sample)
	} else {
		end = t.totalN
	}
	return start, end
}
