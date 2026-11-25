package audioedit

import "fmt"

// Position is one row of the unified sample-position table. Source positions
// are at SourceSampleRate; timeline positions are at the project sample rate.
// Decode positions are zero-based decoded-source sample indices.
type Position struct {
	SegmentID        string `json:"segment_id"`
	SourceID         string `json:"source_id"`
	Format           Format `json:"format"`
	SourceSampleRate int64  `json:"source_sample_rate"`

	TimelineStart int64 `json:"timeline_start"`
	TimelineEnd   int64 `json:"timeline_end"`
	OutputSamples int64 `json:"output_samples"`

	SourceStart int64 `json:"source_start"`
	SourceEnd   int64 `json:"source_end"`

	CodecDelay         int64 `json:"codec_delay"`
	ContentDecodeStart int64 `json:"content_decode_start"`
	ContentDecodeEnd   int64 `json:"content_decode_end"`
	DecodeStart        int64 `json:"decode_start"`
	DecodeEnd          int64 `json:"decode_end"`
	PrerollSamples     int64 `json:"preroll_samples"`
	TailTrimSamples    int64 `json:"tail_trim_samples"`
}

func (p Position) validate() error {
	if p.TimelineEnd <= p.TimelineStart {
		return fmt.Errorf("segment %q has an empty or reversed timeline range", p.SegmentID)
	}
	if p.SourceEnd <= p.SourceStart {
		return fmt.Errorf("segment %q has an empty or reversed source range", p.SegmentID)
	}
	if p.ContentDecodeStart != p.CodecDelay+p.SourceStart {
		return fmt.Errorf("segment %q has inconsistent content decode start", p.SegmentID)
	}
	if p.ContentDecodeEnd != p.CodecDelay+p.SourceEnd {
		return fmt.Errorf("segment %q has inconsistent content decode end", p.SegmentID)
	}
	if p.PrerollSamples < 0 || p.ContentDecodeStart-p.DecodeStart != p.PrerollSamples {
		return fmt.Errorf("segment %q has inconsistent preroll", p.SegmentID)
	}
	if p.TailTrimSamples < 0 || p.DecodeEnd-p.ContentDecodeEnd != p.TailTrimSamples {
		return fmt.Errorf("segment %q has inconsistent tail trim", p.SegmentID)
	}
	return nil
}

// Table is the complete, ordered sample-position plan for a project.
type Table struct {
	SampleRate           int64      `json:"sample_rate"`
	GapSamples           int64      `json:"gap_samples,omitempty"`
	Revision             uint64     `json:"revision"`
	TotalOutputSamples   int64      `json:"total_output_samples"`
	TotalPrerollSamples  int64      `json:"total_preroll_samples"`
	TotalTailTrimSamples int64      `json:"total_tail_trim_samples"`
	Segments             []Position `json:"segments"`
}

// Lookup finds the segment containing an output sample. Gaps between clips
// return false.
func (t Table) Lookup(sample int64) (Position, bool) {
	lo, hi := 0, len(t.Segments)
	for lo < hi {
		mid := lo + (hi-lo)/2
		seg := t.Segments[mid]
		switch {
		case sample < seg.TimelineStart:
			hi = mid
		case sample >= seg.TimelineEnd:
			lo = mid + 1
		default:
			return seg, true
		}
	}
	return Position{}, false
}

// ByID returns the position row for a segment.
func (t Table) ByID(id string) (Position, bool) {
	for _, seg := range t.Segments {
		if seg.SegmentID == id {
			return seg, true
		}
	}
	return Position{}, false
}
