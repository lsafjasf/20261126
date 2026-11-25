package timeline

import (
	"fmt"
	"sort"
)

// Clip describes one timeline event. Sequence and source positions are
// absolute integer frame numbers. Speed is source frames per sequence frame.
type Clip struct {
	SeqIn  int64 `json:"seq_in_frame"`
	SeqOut int64 `json:"seq_out_frame"`
	SrcIn  int64 `json:"src_in_frame"`
	SrcOut int64 `json:"src_out_frame"`
	Speed  Rate  `json:"speed_rate"`
}

func (c Clip) SequenceDuration() int64 {
	return c.SeqOut - c.SeqIn
}

func (c Clip) SourceDuration() int64 {
	return c.SrcOut - c.SrcIn
}

// SourceAt maps a sequence frame inside the clip to a source frame. Generated
// timelines keep this mapping exact at both clip boundaries.
func (c Clip) SourceAt(seq int64) (int64, bool) {
	if seq < c.SeqIn || seq > c.SeqOut {
		return 0, false
	}
	return c.SrcIn + mulDiv(seq-c.SeqIn, c.Speed.Num, c.Speed.Den), true
}

// SequenceAt maps a source frame inside the clip back to a sequence frame.
func (c Clip) SequenceAt(src int64) (int64, bool) {
	if src < c.SrcIn || src > c.SrcOut {
		return 0, false
	}
	return c.SeqIn + mulDiv(src-c.SrcIn, c.Speed.Den, c.Speed.Num), true
}

type Track struct {
	ID    string `json:"id"`
	Clips []Clip `json:"clips"`
}

// Timeline keeps an authoritative duration in frames. Clips may leave gaps and
// may overlap, including within one track.
type Timeline struct {
	Rate     Rate    `json:"rate"`
	Duration int64   `json:"duration_frame"`
	Tracks   []Track `json:"tracks"`
}

func (t Timeline) Validate() error {
	if err := t.Rate.Validate(); err != nil {
		return err
	}
	if t.Duration < 0 {
		return fmt.Errorf("negative timeline duration %d", t.Duration)
	}

	seenTracks := make(map[string]bool, len(t.Tracks))
	for ti := range t.Tracks {
		track := &t.Tracks[ti]
		if track.ID == "" {
			return fmt.Errorf("track %d has an empty id", ti)
		}
		if seenTracks[track.ID] {
			return fmt.Errorf("duplicate track id %q", track.ID)
		}
		seenTracks[track.ID] = true
		for ci := range track.Clips {
			if err := track.Clips[ci].validate(); err != nil {
				return fmt.Errorf("track %q clip %d: %w", track.ID, ci, err)
			}
			if track.Clips[ci].SeqOut > t.Duration {
				return fmt.Errorf(
					"track %q clip %d ends at frame %d beyond timeline duration %d",
					track.ID, ci, track.Clips[ci].SeqOut, t.Duration,
				)
			}
			if ci > 0 {
				prev := track.Clips[ci-1]
				cur := track.Clips[ci]
				if cur.SeqIn < prev.SeqIn ||
					(cur.SeqIn == prev.SeqIn && cur.SeqOut < prev.SeqOut) {
					return fmt.Errorf("track %q clips are not ordered by sequence time", track.ID)
				}
			}
		}
	}
	return nil
}

func (c Clip) validate() error {
	if c.SeqIn < 0 || c.SeqOut <= c.SeqIn {
		return fmt.Errorf("invalid sequence interval [%d,%d)", c.SeqIn, c.SeqOut)
	}
	if c.SrcIn < 0 || c.SrcOut <= c.SrcIn {
		return fmt.Errorf("invalid source interval [%d,%d)", c.SrcIn, c.SrcOut)
	}
	if err := c.Speed.Validate(); err != nil {
		return err
	}

	seqLen := c.SequenceDuration()
	srcLen := c.SourceDuration()
	if got := mulDiv(seqLen, c.Speed.Num, c.Speed.Den); got != srcLen {
		return fmt.Errorf(
			"source duration %d does not match sequence duration %d at speed %s",
			srcLen, seqLen, c.Speed,
		)
	}
	if got := mulDiv(srcLen, c.Speed.Den, c.Speed.Num); got != seqLen {
		return fmt.Errorf(
			"sequence duration %d is not recoverable from source duration %d at speed %s",
			seqLen, srcLen, c.Speed,
		)
	}
	return nil
}

func (t Timeline) Clone() Timeline {
	clone := Timeline{
		Rate:     t.Rate,
		Duration: t.Duration,
		Tracks:   make([]Track, len(t.Tracks)),
	}
	for i := range t.Tracks {
		clone.Tracks[i].ID = t.Tracks[i].ID
		clone.Tracks[i].Clips = append([]Clip(nil), t.Tracks[i].Clips...)
	}
	return clone
}

func (t Timeline) Sort() {
	for i := range t.Tracks {
		sort.SliceStable(t.Tracks[i].Clips, func(a, b int) bool {
			left := t.Tracks[i].Clips[a]
			right := t.Tracks[i].Clips[b]
			if left.SeqIn != right.SeqIn {
				return left.SeqIn < right.SeqIn
			}
			return left.SeqOut < right.SeqOut
		})
	}
}

func mulDiv(value, numerator, denominator int64) int64 {
	if value == 0 || numerator == 0 {
		return 0
	}
	return value * numerator / denominator
}
