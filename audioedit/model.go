package audioedit

import "fmt"

// Params are project-wide edit parameters. Timeline positions are expressed in
// SampleRate samples. GapSamples is inserted between adjacent clips and does
// not add a leading gap before the first clip.
type Params struct {
	SampleRate int64 `json:"sample_rate"`
	GapSamples int64 `json:"gap_samples,omitempty"`
}

// Edit describes the presentation range that a container's edit list exposes.
//
// MediaStart and MediaEnd are zero-based presentation sample positions at the
// source SampleRate. Codec priming is not included in these coordinates: a
// presentation position p maps to decoded position CodecDelay+p. MediaEnd is
// exclusive. A zero MediaEnd means that the end is inferred from other source
// declarations (or from each clip).
type Edit struct {
	MediaStart int64 `json:"media_start,omitempty"`
	MediaEnd   int64 `json:"media_end,omitempty"`
}

// Source is one coded input file and the declarations that describe its coded
// stream. Pointer fields distinguish an explicit zero from "use the format
// default"; for example Opus pre-skip can legitimately be zero.
type Source struct {
	ID         string `json:"id"`
	Format     Format `json:"format"`
	SampleRate int64  `json:"sample_rate"`

	// CodecDelay overrides the format's priming/pre-skip declaration.
	CodecDelay *int64 `json:"codec_delay,omitempty"`
	// FrameSamples overrides the format's coded-frame duration.
	FrameSamples *int64 `json:"frame_samples,omitempty"`
	// EncodedSamples is the total number of decoded samples available in the
	// stream, including priming and trailing codec padding.
	EncodedSamples *int64 `json:"encoded_samples,omitempty"`
	// Packets can be used instead of EncodedSamples. It is multiplied by the
	// resolved frame size.
	Packets *int64 `json:"packets,omitempty"`

	Edit Edit `json:"edit,omitempty"`
}

// Clip selects a presentation range from a source and places it, in order,
// on the output timeline. SourceStart and SourceEnd are presentation samples;
// the end is exclusive. SourceEnd == 0 means the source edit's end.
type Clip struct {
	ID          string `json:"id"`
	SourceID    string `json:"source_id"`
	SourceStart int64  `json:"source_start,omitempty"`
	SourceEnd   int64  `json:"source_end,omitempty"`
}

// Project is a complete declarative edit. Compile derives every position from
// this value; no caller-maintained sample offsets are needed.
type Project struct {
	Params  Params   `json:"params"`
	Sources []Source `json:"sources"`
	Clips   []Clip   `json:"clips"`
}

func (p Params) validate() error {
	if p.SampleRate <= 0 {
		return fmt.Errorf("params.sample_rate must be positive")
	}
	if p.GapSamples < 0 {
		return fmt.Errorf("params.gap_samples must not be negative")
	}
	return nil
}
