package handlefix

import (
	"errors"
	"fmt"
)

// ErrOutOfBounds reports that a trim or repair would leave legal media or
// sequence bounds.
var ErrOutOfBounds = errors.New("range exceeds bounds")

// Range is a half-open integer-frame interval [Start, End).
type Range struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
}

// Len returns the frame count. Invalid reversed ranges report zero.
func (r Range) Len() int64 {
	if r.End <= r.Start {
		return 0
	}
	return r.End - r.Start
}

func (r Range) valid() bool {
	return r.Start <= r.End
}

func (r Range) within(b Range) bool {
	return r.valid() && b.valid() && r.Start >= b.Start && r.End <= b.End
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// extendStart moves the left edge earlier without crossing lower. It returns
// the number of frames actually applied.
func extendStart(r Range, frames, lower int64) (Range, int64) {
	if frames <= 0 || !r.valid() {
		return r, 0
	}
	applied := min64(frames, max64(0, r.Start-lower))
	r.Start -= applied
	return r, applied
}

// extendEnd moves the right edge later without crossing upper.
func extendEnd(r Range, frames, upper int64) (Range, int64) {
	if frames <= 0 || !r.valid() {
		return r, 0
	}
	applied := min64(frames, max64(0, upper-r.End))
	r.End += applied
	return r, applied
}

// trimStartInward moves the left edge later. It is the operation that eats an
// incoming/left handle when called with transition handle length.
func trimStartInward(r Range, frames int64) (Range, error) {
	if frames < 0 {
		return r, fmt.Errorf("negative inward trim %d", frames)
	}
	r.Start += frames
	if !r.valid() {
		return r, fmt.Errorf("%w: start trim crosses end", ErrOutOfBounds)
	}
	return r, nil
}

// trimEndInward moves the right edge earlier. It eats an outgoing/right handle.
func trimEndInward(r Range, frames int64) (Range, error) {
	if frames < 0 {
		return r, fmt.Errorf("negative inward trim %d", frames)
	}
	r.End -= frames
	if !r.valid() {
		return r, fmt.Errorf("%w: end trim crosses start", ErrOutOfBounds)
	}
	return r, nil
}
