package ttmlevents

import "time"

// Interval is a half-open TTML active interval [Begin, *End). A nil End
// means an unbounded interval.
type Interval struct {
	Begin time.Duration  `json:"begin"`
	End   *time.Duration `json:"end,omitempty"`
}

// BoundedInterval creates a finite interval.
func BoundedInterval(begin, end time.Duration) Interval {
	e := end
	return Interval{Begin: begin, End: &e}
}

// UnboundedInterval creates an interval with no finite end.
func UnboundedInterval(begin time.Duration) Interval {
	return Interval{Begin: begin}
}

// EndValue returns the finite end or fallback when End is nil.
func (i Interval) EndValue(fallback time.Duration) time.Duration {
	if i.End == nil {
		return fallback
	}
	return *i.End
}

// Finite reports whether the interval has an end.
func (i Interval) Finite() bool { return i.End != nil }

// Empty reports whether the half-open interval has no positive duration.
func (i Interval) Empty() bool {
	return i.End != nil && *i.End <= i.Begin
}

// Contains reports whether t belongs to [Begin,End).
func (i Interval) Contains(t time.Duration) bool {
	if t < i.Begin {
		return false
	}
	return i.End == nil || t < *i.End
}

// Clip returns the intersection of two intervals. A zero-duration
// intersection is retained with ok=true when the boundaries merely touch;
// callers that render text should treat Empty() as non-visible.
func (i Interval) Clip(other Interval) (Interval, bool) {
	begin := i.Begin
	if other.Begin > begin {
		begin = other.Begin
	}

	var end *time.Duration
	switch {
	case i.End == nil:
		if other.End != nil {
			e := *other.End
			end = &e
		}
	case other.End == nil:
		e := *i.End
		end = &e
	default:
		e := *i.End
		if *other.End < e {
			e = *other.End
		}
		end = &e
	}

	if end != nil && *end < begin {
		return Interval{}, false
	}
	return Interval{Begin: begin, End: end}, true
}

func minDurationPtr(a, b *time.Duration) *time.Duration {
	if a == nil {
		return b
	}
	if b == nil || *a <= *b {
		return a
	}
	return b
}

func cloneDurationPtr(v *time.Duration) *time.Duration {
	if v == nil {
		return nil
	}
	x := *v
	return &x
}
