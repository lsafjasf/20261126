package timeline

import (
	"fmt"
	"strconv"
	"strings"
)

// Rate is an exact rational number. Timeline rates are frames per second and
// clip speeds are source frames per sequence frame.
type Rate struct {
	Num int64
	Den int64
}

func NewRate(num, den int64) (Rate, error) {
	r := Rate{Num: num, Den: den}
	if err := r.Validate(); err != nil {
		return Rate{}, err
	}
	return r, nil
}

func (r Rate) Validate() error {
	if r.Num <= 0 || r.Den <= 0 {
		return fmt.Errorf("rate must be positive, got %d/%d", r.Num, r.Den)
	}
	return nil
}

func ParseRate(s string) (Rate, error) {
	s = strings.TrimSpace(s)
	parts := strings.Split(s, "/")
	if len(parts) != 2 {
		return Rate{}, fmt.Errorf("invalid rate %q: want num/den", s)
	}
	num, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return Rate{}, fmt.Errorf("invalid rate numerator %q: %w", parts[0], err)
	}
	den, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return Rate{}, fmt.Errorf("invalid rate denominator %q: %w", parts[1], err)
	}
	return NewRate(num, den)
}

func (r Rate) String() string {
	return fmt.Sprintf("%d/%d", r.Num, r.Den)
}

func gcd(a, b int64) int64 {
	if a < 0 {
		a = -a
	}
	if b < 0 {
		b = -b
	}
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

// nominalFPS is the frame-number clock used by SMPTE timecode. Fractional
// NTSC rates use their nearest integer (29.97 -> 30, 23.976 -> 24).
func (r Rate) nominalFPS() int64 {
	return (r.Num + r.Den/2) / r.Den
}

func (r Rate) dropFrames() int64 {
	switch {
	case r.Num*1001 == 30000*r.Den:
		return 2
	case r.Num*1001 == 60000*r.Den:
		return 4
	default:
		return 0
	}
}

func (r Rate) isDropFrame() bool {
	return r.dropFrames() != 0
}

func (r Rate) framesPerMinute() int64 {
	return r.nominalFPS()*60 - r.dropFrames()
}

func (r Rate) framesPer10Minutes() int64 {
	return r.nominalFPS()*600 - 9*r.dropFrames()
}

func (r Rate) framesPerHour() int64 {
	return r.nominalFPS()*3600 - 54*r.dropFrames()
}

func (r Rate) framesPerDay() int64 {
	return r.framesPerHour() * 24
}
