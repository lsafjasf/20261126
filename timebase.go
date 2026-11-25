package subselect

import (
	"errors"
	"time"
)

// ErrInvalidTimeBase is returned when a track declares a zero timescale or
// cues that cannot be rebased onto the canonical playback timeline.
var ErrInvalidTimeBase = errors.New("subselect: invalid time base")

// TimeBase maps the integer timestamp units stored on a Track onto the
// single canonical movie timeline shared by playback sessions.
//
// Canonical position is:
//
//	position = units / Timescale seconds + Offset
//
// Different subtitle versions of the same language frequently use different
// timescales (1000, 48000, 90000, ...) or carry an edit-list / encoder delay
// Offset. Copying cue text without re-running this conversion is the
// "new language, old timing" defect.
type TimeBase struct {
	// Timescale is the number of track units per second; it must be > 0.
	Timescale uint32

	// Offset is added after scaling. It captures track start offsets such
	// as MP4 edit lists or composition start time.
	Offset time.Duration
}

// ToDuration converts one track-local timestamp value to canonical position.
// The arithmetic keeps the fractional remainder away from float rounding.
func (tb TimeBase) ToDuration(units int64) time.Duration {
	ts := int64(tb.Timescale)
	if ts <= 0 {
		return 0
	}

	whole := units / ts
	remainder := units % ts
	if remainder < 0 {
		whole--
		remainder += ts
	}

	// remainder is in [0, ts); remainder*time.Second stays inside int64
	// for every uint32 timescale because (2^32-1)*1e9 < 2^63.
	fractionNanos := (remainder * int64(time.Second)) / ts
	return time.Duration(whole)*time.Second +
		time.Duration(fractionNanos) +
		tb.Offset
}

// FromDuration performs the inverse mapping (canonical position -> track
// units). It is mainly useful for fixtures and tests.
func (tb TimeBase) FromDuration(pos time.Duration) int64 {
	ts := int64(tb.Timescale)
	if ts <= 0 {
		return 0
	}
	shifted := pos - tb.Offset
	nanos := int64(shifted)
	wholeSeconds := nanos / int64(time.Second)
	remainderNanos := nanos % int64(time.Second)
	if remainderNanos < 0 {
		wholeSeconds--
		remainderNanos += int64(time.Second)
	}
	return wholeSeconds*ts +
		(remainderNanos*ts)/int64(time.Second)
}
