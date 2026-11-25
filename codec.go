package timeline

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

const formatVersion = 1

type document struct {
	Version       int        `json:"version"`
	Rate          string     `json:"rate"`
	DurationFrame int64      `json:"duration_frame"`
	Tracks        []docTrack `json:"tracks"`
}

type docTrack struct {
	ID    string    `json:"id"`
	Clips []docClip `json:"clips"`
}

type docClip struct {
	SeqIn  string `json:"seq_in"`
	SeqOut string `json:"seq_out"`
	SrcIn  string `json:"src_in"`
	SrcOut string `json:"src_out"`
	Speed  string `json:"speed"`
}

// Marshal serializes a timeline using SMPTE timecode for every clip boundary.
// The timeline duration is kept as an exact frame count.
func Marshal(t Timeline) ([]byte, error) {
	return marshalWith(t, FrameToTimecode)
}

type timecodeEncoder func(Rate, int64) (string, error)

func marshalWith(t Timeline, encode timecodeEncoder) ([]byte, error) {
	if err := t.Validate(); err != nil {
		return nil, err
	}
	doc := document{
		Version:       formatVersion,
		Rate:          t.Rate.String(),
		DurationFrame: t.Duration,
		Tracks:        make([]docTrack, len(t.Tracks)),
	}
	for ti := range t.Tracks {
		doc.Tracks[ti].ID = t.Tracks[ti].ID
		doc.Tracks[ti].Clips = make([]docClip, len(t.Tracks[ti].Clips))
		for ci := range t.Tracks[ti].Clips {
			clip := t.Tracks[ti].Clips[ci]
			seqIn, err := encode(t.Rate, clip.SeqIn)
			if err != nil {
				return nil, fmt.Errorf("encode track %q clip %d sequence in: %w", t.Tracks[ti].ID, ci, err)
			}
			seqOut, err := encode(t.Rate, clip.SeqOut)
			if err != nil {
				return nil, fmt.Errorf("encode track %q clip %d sequence out: %w", t.Tracks[ti].ID, ci, err)
			}
			srcIn, err := encode(t.Rate, clip.SrcIn)
			if err != nil {
				return nil, fmt.Errorf("encode track %q clip %d source in: %w", t.Tracks[ti].ID, ci, err)
			}
			srcOut, err := encode(t.Rate, clip.SrcOut)
			if err != nil {
				return nil, fmt.Errorf("encode track %q clip %d source out: %w", t.Tracks[ti].ID, ci, err)
			}
			doc.Tracks[ti].Clips[ci] = docClip{
				SeqIn:  seqIn,
				SeqOut: seqOut,
				SrcIn:  srcIn,
				SrcOut: srcOut,
				Speed:  clip.Speed.String(),
			}
		}
	}

	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal timeline: %w", err)
	}
	return append(data, '\n'), nil
}

// Unmarshal reads the timecode interchange representation and validates that
// every clip remains inside the declared timeline duration.
func Unmarshal(data []byte) (Timeline, error) {
	var doc document
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&doc); err != nil {
		return Timeline{}, fmt.Errorf("decode timeline: %w", err)
	}
	if err := ensureEOF(decoder); err != nil {
		return Timeline{}, err
	}
	if doc.Version != formatVersion {
		return Timeline{}, fmt.Errorf("unsupported timeline version %d", doc.Version)
	}

	rate, err := ParseRate(doc.Rate)
	if err != nil {
		return Timeline{}, fmt.Errorf("decode rate: %w", err)
	}
	timeline := Timeline{
		Rate:     rate,
		Duration: doc.DurationFrame,
		Tracks:   make([]Track, len(doc.Tracks)),
	}
	for ti := range doc.Tracks {
		timeline.Tracks[ti].ID = doc.Tracks[ti].ID
		timeline.Tracks[ti].Clips = make([]Clip, len(doc.Tracks[ti].Clips))
		for ci := range doc.Tracks[ti].Clips {
			clip, err := decodeClip(rate, doc.Tracks[ti].Clips[ci])
			if err != nil {
				return Timeline{}, fmt.Errorf("decode track %q clip %d: %w", doc.Tracks[ti].ID, ci, err)
			}
			timeline.Tracks[ti].Clips[ci] = clip
		}
	}
	if err := timeline.Validate(); err != nil {
		return Timeline{}, fmt.Errorf("validate decoded timeline: %w", err)
	}
	return timeline, nil
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("decode timeline: multiple JSON values")
		}
		return fmt.Errorf("decode timeline: %w", err)
	}
	return nil
}

func decodeClip(rate Rate, doc docClip) (Clip, error) {
	seqIn, err := ParseTimecode(rate, doc.SeqIn)
	if err != nil {
		return Clip{}, fmt.Errorf("parse sequence in: %w", err)
	}
	seqOut, err := ParseTimecode(rate, doc.SeqOut)
	if err != nil {
		return Clip{}, fmt.Errorf("parse sequence out: %w", err)
	}
	srcIn, err := ParseTimecode(rate, doc.SrcIn)
	if err != nil {
		return Clip{}, fmt.Errorf("parse source in: %w", err)
	}
	srcOut, err := ParseTimecode(rate, doc.SrcOut)
	if err != nil {
		return Clip{}, fmt.Errorf("parse source out: %w", err)
	}
	speed, err := ParseRate(doc.Speed)
	if err != nil {
		return Clip{}, fmt.Errorf("parse speed: %w", err)
	}
	return Clip{
		SeqIn:  seqIn,
		SeqOut: seqOut,
		SrcIn:  srcIn,
		SrcOut: srcOut,
		Speed:  speed,
	}, nil
}
