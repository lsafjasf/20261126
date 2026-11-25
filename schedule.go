package subselect

import (
	"sort"
	"time"
)

// TimedCue is a cue fully mapped onto the canonical movie timeline.
// Instances are immutable and safe to share between goroutines.
type TimedCue struct {
	ID    string        `json:"id"`
	Start time.Duration `json:"start_ns"`
	End   time.Duration `json:"end_ns"`
	Text  string        `json:"text"`

	TrackID  string `json:"track_id"`
	Language string `json:"language"`
	Version  string `json:"version,omitempty"`
}

// StartMS returns Start in whole milliseconds.
func (q TimedCue) StartMS() int64 { return q.Start.Milliseconds() }

// EndMS returns End in whole milliseconds.
func (q TimedCue) EndMS() int64 { return q.End.Milliseconds() }

// Contains reports whether pos lies in the half-open cue interval.
func (q TimedCue) Contains(pos time.Duration) bool {
	return pos >= q.Start && pos < q.End
}

// Schedule is the immutable result of rebasing one track's cues. A view
// publishes the schedule and its track identity together, so observers can
// never pair a new language's text with an old track's timings.
type Schedule struct {
	TrackID  string     `json:"track_id"`
	Language string     `json:"language"`
	Version  string     `json:"version,omitempty"`
	Cues     []TimedCue `json:"cues"`
}

// Rebase maps every cue through track.TimeBase and returns a sorted schedule.
// The returned cue slice is independent of the input slice.
func Rebase(track Track, cues []Cue) (*Schedule, error) {
	if track.TimeBase.Timescale == 0 {
		return nil, ErrInvalidTimeBase
	}

	rebased := make([]TimedCue, 0, len(cues))
	for _, cue := range cues {
		start := track.TimeBase.ToDuration(cue.Start)
		end := track.TimeBase.ToDuration(cue.End)
		rebased = append(rebased, TimedCue{
			ID:       cue.ID,
			Start:    start,
			End:      end,
			Text:     cue.Text,
			TrackID:  track.ID,
			Language: track.Language,
			Version:  track.Version,
		})
	}

	sort.SliceStable(rebased, func(i, j int) bool {
		if rebased[i].Start != rebased[j].Start {
			return rebased[i].Start < rebased[j].Start
		}
		return rebased[i].End < rebased[j].End
	})

	return &Schedule{
		TrackID:  track.ID,
		Language: track.Language,
		Version:  track.Version,
		Cues:     rebased,
	}, nil
}

// Active returns the cue covering pos. Cue intervals are half-open and the
// latest starting cue at or before pos wins (standard subtitle behaviour).
func (s *Schedule) Active(pos time.Duration) (TimedCue, bool) {
	if s == nil || len(s.Cues) == 0 {
		return TimedCue{}, false
	}

	idx := sort.Search(len(s.Cues), func(i int) bool {
		return s.Cues[i].Start > pos
	}) - 1
	if idx < 0 {
		return TimedCue{}, false
	}
	cue := s.Cues[idx]
	if !cue.Contains(pos) {
		return TimedCue{}, false
	}
	return cue, true
}
