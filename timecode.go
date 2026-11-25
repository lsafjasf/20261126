package sdh

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ErrBadTimestamp is returned when a timecode cannot be parsed.
var ErrBadTimestamp = errors.New("sdh: bad timestamp")

// ParseTimestamp parses an SRT-style "HH:MM:SS,mmm" (comma or dot
// fractional separator, any fractional digit count) into a Duration.
func ParseTimestamp(s string) (time.Duration, error) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return 0, fmt.Errorf("%w: empty", ErrBadTimestamp)
	}
	// Split off fractional seconds.
	main := trimmed
	frac := ""
	if i := strings.IndexAny(trimmed, ".,"); i >= 0 {
		main = trimmed[:i]
		frac = trimmed[i+1:]
		// A second colon after the separator means we matched a decimal
		// separator incorrectly; SRT never does that.
		if strings.Contains(frac, ":") {
			return 0, fmt.Errorf("%w: %q", ErrBadTimestamp, s)
		}
	}
	parts := strings.Split(main, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, fmt.Errorf("%w: %q", ErrBadTimestamp, s)
	}
	var hours, mins, secs int
	var err error
	if len(parts) == 3 {
		if hours, err = strconv.Atoi(parts[0]); err != nil {
			return 0, fmt.Errorf("%w: %q", ErrBadTimestamp, s)
		}
		if mins, err = strconv.Atoi(parts[1]); err != nil {
			return 0, fmt.Errorf("%w: %q", ErrBadTimestamp, s)
		}
		if secs, err = strconv.Atoi(parts[2]); err != nil {
			return 0, fmt.Errorf("%w: %q", ErrBadTimestamp, s)
		}
	} else {
		if mins, err = strconv.Atoi(parts[0]); err != nil {
			return 0, fmt.Errorf("%w: %q", ErrBadTimestamp, s)
		}
		if secs, err = strconv.Atoi(parts[1]); err != nil {
			return 0, fmt.Errorf("%w: %q", ErrBadTimestamp, s)
		}
	}
	if mins < 0 || mins > 59 || secs < 0 || secs > 59 || hours < 0 {
		return 0, fmt.Errorf("%w: %q", ErrBadTimestamp, s)
	}

	d := time.Duration(hours)*time.Hour +
		time.Duration(mins)*time.Minute +
		time.Duration(secs)*time.Second

	if frac != "" {
		if len(frac) > 9 {
			frac = frac[:9]
		}
		n, err := strconv.Atoi(frac)
		if err != nil {
			return 0, fmt.Errorf("%w: %q", ErrBadTimestamp, s)
		}
		// Scale the fractional value to nanoseconds: frac has len(frac)
		// decimal places.
		scale := 1
		for i := 0; i < len(frac); i++ {
			scale *= 10
		}
		d += time.Duration(n) * time.Second / time.Duration(scale)
	}
	return d, nil
}

// FormatSRTTime formats d as "HH:MM:SS,mmm".
func FormatSRTTime(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	ms := int64(d / time.Millisecond)
	hours := ms / 3_600_000
	ms -= hours * 3_600_000
	mins := ms / 60_000
	ms -= mins * 60_000
	secs := ms / 1000
	millis := ms - secs*1000
	return fmt.Sprintf("%02d:%02d:%02d,%03d", hours, mins, secs, millis)
}

// FormatASSTime formats d as ASS style "H:MM:SS.cc" (centiseconds).
func FormatASSTime(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	cs := int64(d / (10 * time.Millisecond))
	hours := cs / 360_000
	cs -= hours * 360_000
	mins := cs / 6000
	cs -= mins * 6000
	secs := cs / 100
	centis := cs - secs*100
	return fmt.Sprintf("%d:%02d:%02d.%02d", hours, mins, secs, centis)
}
