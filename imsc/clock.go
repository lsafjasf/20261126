package imsc

import (
	"strconv"
	"strings"
	"time"
)

// defaultFrameRate is applied to four-component clock expressions that carry
// frames but no subframes (HH:MM:SS:FF).
const defaultFrameRate = 30

var offsetUnits = []struct {
	suffix string
	unit   time.Duration
}{
	{"h", time.Hour},
	{"ms", time.Millisecond},
	{"s", time.Second},
	{"m", time.Minute},
}

// parseClock parses a TTML begin/end expression. It supports offset-time
// values (for example "1.5s", "200ms") and clock-time values
// ("HH:MM:SS", "HH:MM:SS.fraction", "HH:MM:SS:FF").
func parseClock(expr string) (time.Duration, error) {
	return parseClockWithRate(expr, defaultFrameRate)
}

// parseClockWithRate behaves like parseClock but uses the supplied frame rate
// for four-component HH:MM:SS:FF expressions.
func parseClockWithRate(expr string, frameRate int) (time.Duration, error) {
	value := strings.TrimSpace(expr)
	if value == "" {
		return 0, &clockError{expr: expr, detail: "empty clock expression"}
	}

	if d, ok := parseOffsetTime(value); ok {
		return d, nil
	}
	if d, ok := parseClockTime(value, frameRate); ok {
		return d, nil
	}
	return 0, &clockError{expr: expr, detail: "not a valid offset-time or clock-time expression"}
}

type clockError struct {
	expr   string
	detail string
}

func (e *clockError) Error() string {
	return "invalid clock expression " + strconv.Quote(e.expr) + ": " + e.detail
}

func parseOffsetTime(value string) (time.Duration, bool) {
	for _, u := range offsetUnits {
		if strings.HasSuffix(value, u.suffix) {
			num := strings.TrimSuffix(value, u.suffix)
			f, err := strconv.ParseFloat(num, 64)
			if err != nil || f < 0 {
				return 0, false
			}
			return time.Duration(f * float64(u.unit)), true
		}
	}
	return 0, false
}

func parseClockTime(value string, frameRate int) (time.Duration, bool) {
	parts := strings.Split(value, ":")
	if len(parts) != 3 && len(parts) != 4 {
		return 0, false
	}

	hours, err := strconv.Atoi(parts[0])
	if err != nil || hours < 0 {
		return 0, false
	}
	minutes, err := strconv.Atoi(parts[1])
	if err != nil || minutes < 0 || minutes > 59 {
		return 0, false
	}

	var seconds, fraction float64
	if len(parts) == 4 {
		secs, err := strconv.Atoi(parts[2])
		if err != nil || secs < 0 || secs > 59 {
			return 0, false
		}
		frames, err := strconv.Atoi(parts[3])
		if err != nil || frames < 0 || frames >= frameRate {
			return 0, false
		}
		seconds = float64(secs) + float64(frames)/float64(frameRate)
	} else {
		secParts := strings.FieldsFunc(parts[2], func(r rune) bool {
			return r == '.' || r == ','
		})
		secs, err := strconv.Atoi(secParts[0])
		if err != nil || secs < 0 || secs > 59 {
			return 0, false
		}
		seconds = float64(secs)
		if len(secParts) == 2 {
			f, err := strconv.ParseFloat("0."+secParts[1], 64)
			if err != nil || f < 0 {
				return 0, false
			}
			fraction = f
		} else if len(secParts) > 2 {
			return 0, false
		}
	}

	total := time.Duration(hours)*time.Hour +
		time.Duration(minutes)*time.Minute +
		time.Duration((seconds+fraction)*float64(time.Second))
	return total, true
}
