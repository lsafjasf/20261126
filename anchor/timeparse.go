package anchor

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseTime converts a user-supplied timestamp into ticks.
//
// Accepted forms:
//   - plain integer: ticks ("15000000")
//   - seconds with unit suffix: "12s", "12.5s", "250ms", "1500us"
//   - wall-clock: "HH:MM:SS" or "HH:MM:SS.mmm" (millisecond precision)
//
// Fractional results that would need sub-tick precision are rejected rather
// than silently rounded, keeping all stored values exact integers.
func (p *Project) ParseTime(s string) (Ticks, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty timestamp")
	}
	if strings.ContainsAny(s, ":") {
		return p.parseClock(s)
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return Ticks(n), nil
	}
	return p.parseSuffixed(s)
}

func (p *Project) parseSuffixed(s string) (Ticks, error) {
	multipliers := []struct {
		suffix string
		scale  int64
	}{
		{"us", 1},
		{"ms", 1000},
		{"s", 1000 * 1000},
	}
	for _, m := range multipliers {
		if !strings.HasSuffix(s, m.suffix) {
			continue
		}
		num := strings.TrimSuffix(s, m.suffix)
		value, err := strconv.ParseFloat(num, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid timestamp %q", s)
		}
		micros := value * float64(m.scale)
		ticks := micros * float64(p.TicksPerSecond) / 1e6
		rounded := Ticks(ticks + 0.5)
		delta := ticks - float64(rounded)
		if delta > 1e-9 || delta < -1e-9 {
			return 0, fmt.Errorf("timestamp %q needs sub-tick precision", s)
		}
		return rounded, nil
	}
	return 0, fmt.Errorf("unrecognized timestamp %q (use ticks, s, ms, us or HH:MM:SS.mmm)", s)
}

func (p *Project) parseClock(s string) (Ticks, error) {
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return 0, fmt.Errorf("clock timestamp must be HH:MM:SS[.mmm]")
	}
	hours, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid hours in %q", s)
	}
	mins, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || mins < 0 || mins > 59 {
		return 0, fmt.Errorf("invalid minutes in %q", s)
	}
	secParts := strings.Split(parts[2], ".")
	if len(secParts) > 2 {
		return 0, fmt.Errorf("invalid seconds in %q", s)
	}
	secs, err := strconv.ParseInt(secParts[0], 10, 64)
	if err != nil || secs < 0 || secs > 59 {
		return 0, fmt.Errorf("invalid seconds in %q", s)
	}
	var millis int64
	if len(secParts) == 2 {
		frac := secParts[1]
		if len(frac) > 3 {
			return 0, fmt.Errorf("clock timestamp supports millisecond precision only")
		}
		millis, err = strconv.ParseInt(frac+strings.Repeat("0", 3-len(frac)), 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid milliseconds in %q", s)
		}
	}
	totalMicros := (hours*3600+mins*60+secs)*1e6 + millis*1000
	ticks := Ticks(totalMicros) * Ticks(p.TicksPerSecond) / 1_000_000
	if (Ticks(totalMicros)*Ticks(p.TicksPerSecond))%1_000_000 != 0 {
		return 0, fmt.Errorf("clock timestamp %q needs sub-tick precision", s)
	}
	return ticks, nil
}
