package smptett

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// parseTTMLTime parses SMPTE-TT clock times and offset times.
func parseTTMLTime(value string, frameRate, tickRate float64) (time.Duration, error) {
	s := strings.TrimSpace(value)
	if s == "" {
		return 0, fmt.Errorf("empty time expression")
	}
	if strings.ContainsAny(s, "TZ") && strings.Contains(s, "-") {
		return 0, fmt.Errorf("wall-clock time %q is not supported", value)
	}

	if strings.Contains(s, ":") {
		return parseClockTime(s, frameRate)
	}

	units := []struct {
		suffix string
		scale  float64
	}{
		{"ms", float64(time.Millisecond)},
		{"h", float64(time.Hour)},
		{"m", float64(time.Minute)},
		{"s", float64(time.Second)},
		{"f", float64(time.Second) / frameRate},
		{"t", float64(time.Second) / tickRate},
	}
	for _, unit := range units {
		if !strings.HasSuffix(s, unit.suffix) {
			continue
		}
		raw := strings.TrimSpace(strings.TrimSuffix(s, unit.suffix))
		n, err := strconv.ParseFloat(raw, 64)
		if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 {
			return 0, fmt.Errorf("invalid time expression %q", value)
		}
		return time.Duration(math.Round(n * unit.scale)), nil
	}
	return 0, fmt.Errorf("invalid time expression %q (a unit such as s, ms, f, or t is required)", value)
}

func parseClockTime(s string, frameRate float64) (time.Duration, error) {
	parts := strings.Split(s, ":")
	if len(parts) != 3 && len(parts) != 4 {
		return 0, fmt.Errorf("invalid clock time %q", s)
	}
	if len(parts) == 4 && frameRate <= 0 {
		return 0, fmt.Errorf("frame clock time %q requires a frame rate", s)
	}

	hours, err := strconv.ParseFloat(parts[0], 64)
	if err != nil || hours < 0 {
		return 0, fmt.Errorf("invalid clock time %q", s)
	}
	minutes, err := strconv.ParseFloat(parts[1], 64)
	if err != nil || minutes < 0 || minutes >= 60 {
		return 0, fmt.Errorf("invalid clock time %q", s)
	}
	seconds, err := strconv.ParseFloat(parts[2], 64)
	if err != nil || seconds < 0 || seconds >= 60 {
		return 0, fmt.Errorf("invalid clock time %q", s)
	}

	total := hours*float64(time.Hour) + minutes*float64(time.Minute) + seconds*float64(time.Second)
	if len(parts) == 4 {
		frames, err := strconv.ParseFloat(parts[3], 64)
		if err != nil || frames < 0 || frames >= frameRate {
			return 0, fmt.Errorf("invalid frame component in clock time %q", s)
		}
		total += frames * float64(time.Second) / frameRate
	}
	return time.Duration(math.Round(total)), nil
}

func parseFrameRate(value, multiplier string) float64 {
	rate := 30.0
	if n, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err == nil && n > 0 {
		rate = n
	}
	if multiplier == "" {
		return rate
	}
	parts := strings.Fields(multiplier)
	if len(parts) != 2 {
		return rate
	}
	num, err1 := strconv.ParseFloat(parts[0], 64)
	den, err2 := strconv.ParseFloat(parts[1], 64)
	if err1 == nil && err2 == nil && num > 0 && den > 0 {
		rate *= num / den
	}
	return rate
}

func formatTimestamp(t time.Duration) string {
	if t < 0 {
		t = 0
	}
	ms := t.Milliseconds()
	return fmt.Sprintf("%03d:%02d:%02d.%03d",
		ms/3_600_000,
		(ms/60_000)%60,
		(ms/1_000)%60,
		ms%1_000,
	)
}

// ParseTimes parses a comma-separated TTML time list.
func ParseTimes(value string, frameRate, tickRate float64) ([]time.Duration, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	if frameRate <= 0 {
		frameRate = 30
	}
	if tickRate <= 0 {
		tickRate = frameRate
	}
	var out []time.Duration
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		d, err := parseTTMLTime(part, frameRate, tickRate)
		if err != nil {
			return nil, fmt.Errorf("time %q: %w", part, err)
		}
		out = append(out, d)
	}
	return out, nil
}
