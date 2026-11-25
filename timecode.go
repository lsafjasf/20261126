package ttmlevents

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// ParseTime parses the common TTML offset/clock forms using 30 fps for frame
// counts. Use Document.parseTime for document-declared frame/tick rates.
func ParseTime(raw string) (time.Duration, error) {
	return parseTime(raw, 30, 0)
}

func (d *Document) parseTime(raw string) (time.Duration, bool, error) {
	raw = strings.TrimSpace(raw)
	if strings.EqualFold(raw, "indefinite") {
		return 0, true, nil
	}
	frames := 30.0
	ticks := 0.0
	if d != nil {
		if d.FrameRate > 0 {
			frames = d.FrameRate
		}
		ticks = d.TickRate
	}
	value, err := parseTime(raw, frames, ticks)
	return value, false, err
}

func parseTime(raw string, frameRate, tickRate float64) (time.Duration, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, fmt.Errorf("empty time expression")
	}
	if strings.EqualFold(s, "indefinite") {
		return 0, fmt.Errorf("indefinite time")
	}

	if strings.Contains(s, ":") {
		return parseClockTime(s, frameRate)
	}

	numberPart, unit := splitTimeUnit(s)
	value, err := strconv.ParseFloat(numberPart, 64)
	if err != nil {
		return 0, fmt.Errorf("bad time %q: %w", raw, err)
	}
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return 0, fmt.Errorf("invalid time %q", raw)
	}

	switch strings.ToLower(unit) {
	case "h", "hour", "hours":
		return durationFromSeconds(value * 3600), nil
	case "m", "min", "minute", "minutes":
		return durationFromSeconds(value * 60), nil
	case "s", "sec", "second", "seconds", "":
		return durationFromSeconds(value), nil
	case "ms", "millisecond", "milliseconds":
		return time.Duration(math.Round(value * float64(time.Millisecond))), nil
	case "f", "frame", "frames":
		if frameRate <= 0 {
			return 0, fmt.Errorf("frame time without frameRate: %q", raw)
		}
		return durationFromSeconds(value / frameRate), nil
	case "t", "tick", "ticks":
		if tickRate <= 0 {
			return 0, fmt.Errorf("tick time without tickRate: %q", raw)
		}
		return durationFromSeconds(value / tickRate), nil
	default:
		return 0, fmt.Errorf("unsupported time unit %q in %q", unit, raw)
	}
}

func parseClockTime(s string, frameRate float64) (time.Duration, error) {
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 4 {
		return 0, fmt.Errorf("bad clock time %q", s)
	}

	numbers := make([]float64, len(parts))
	for i, part := range parts {
		v, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil || v < 0 {
			return 0, fmt.Errorf("bad clock component %q in %q", part, s)
		}
		numbers[i] = v
	}

	var hours, minutes, seconds float64
	var frames float64
	switch len(parts) {
	case 2: // mm:ss[.fraction]
		minutes, seconds = numbers[0], numbers[1]
	case 3:
		// hh:mm:ss.fraction is the usual clock form. A final integer after
		// hh:mm:ss may also denote frames when explicitly colon-separated by
		// profiles that use hh:mm:ss:ff; that has four components below.
		hours, minutes, seconds = numbers[0], numbers[1], numbers[2]
	case 4: // hh:mm:ss:frames[.subframes]
		hours, minutes, seconds, frames = numbers[0], numbers[1], numbers[2], numbers[3]
	}

	total := hours*3600 + minutes*60 + seconds
	d := durationFromSeconds(total)
	if frames != 0 {
		if frameRate <= 0 {
			return 0, fmt.Errorf("frame clock without frameRate: %q", s)
		}
		d += durationFromSeconds(frames / frameRate)
	}
	return d, nil
}

func splitTimeUnit(s string) (number, unit string) {
	i := len(s)
	for i > 0 {
		r := s[i-1]
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '%' {
			i--
			continue
		}
		break
	}
	if i == len(s) {
		return strings.TrimSpace(s), "s"
	}
	return strings.TrimSpace(s[:i]), strings.ToLower(strings.TrimSpace(s[i:]))
}

func durationFromSeconds(seconds float64) time.Duration {
	return time.Duration(math.Round(seconds * float64(time.Second)))
}

func formatDuration(d time.Duration) string {
	if d == 0 {
		return "0s"
	}
	ms := float64(d) / float64(time.Millisecond)
	if ms == math.Trunc(ms) {
		if d%time.Second == 0 {
			return strconv.FormatInt(int64(d/time.Second), 10) + "s"
		}
		return strconv.FormatInt(int64(ms), 10) + "ms"
	}
	return strconv.FormatFloat(float64(d)/float64(time.Second), 'f', -1, 64) + "s"
}

func durationSeconds(d time.Duration) float64 {
	return float64(d) / float64(time.Second)
}
