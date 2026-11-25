package edlframe

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Timecode is a parsed SMPTE-style timecode. Frame is the linear frame index
// from midnight after drop-frame correction.
type Timecode struct {
	Hour     int
	Minute   int
	Second   int
	Frame    int
	Linear   int64
	Timebase Timebase
}

var timecodePattern = regexp.MustCompile(`^(\d{1,3})[:;.,](\d{1,2})[:;.,](\d{1,2})[:;.,](\d{1,3})$`)

// ParseTimecode parses HH:MM:SS:FF using the supplied timebase.
func ParseTimecode(raw string, tb Timebase) (Timecode, error) {
	text := strings.TrimSpace(raw)
	matches := timecodePattern.FindStringSubmatch(text)
	if matches == nil {
		return Timecode{}, fmt.Errorf("invalid timecode %q", raw)
	}

	values := make([]int, 4)
	for i := 1; i <= 4; i++ {
		value, err := strconv.Atoi(matches[i])
		if err != nil {
			return Timecode{}, fmt.Errorf("invalid timecode %q", raw)
		}
		values[i-1] = value
	}
	hour, minute, second, frame := values[0], values[1], values[2], values[3]

	if hour < 0 || hour > 23 {
		return Timecode{}, fmt.Errorf("timecode hour out of range in %q", raw)
	}
	if minute < 0 || minute > 59 || second < 0 || second > 59 {
		return Timecode{}, fmt.Errorf("timecode minute/second out of range in %q", raw)
	}
	if frame < 0 || frame >= tb.TimecodeBase {
		return Timecode{}, fmt.Errorf("frame %d out of range for %dfps in %q", frame, tb.TimecodeBase, raw)
	}

	drop := 0
	totalMinutes := int64(hour)*60 + int64(minute)
	if tb.DropFrame {
		if tb.TimecodeBase != 30 && tb.TimecodeBase != 60 {
			return Timecode{}, fmt.Errorf("unsupported drop-frame base %d", tb.TimecodeBase)
		}
		drop = dropFrameIncrement(tb.TimecodeBase)
		if minute%10 != 0 && second == 0 && frame < drop {
			return Timecode{}, fmt.Errorf("dropped timecode frame %q", raw)
		}
	}

	linear := ((int64(hour)*60+int64(minute))*60+int64(second))*int64(tb.TimecodeBase) + int64(frame)
	if tb.DropFrame {
		dropped := int64(drop) * (totalMinutes - totalMinutes/10)
		linear -= dropped
	}

	return Timecode{
		Hour:     hour,
		Minute:   minute,
		Second:   second,
		Frame:    frame,
		Linear:   linear,
		Timebase: tb,
	}, nil
}

// TimecodeParts splits a possibly multi-day linear frame index.
func TimecodeParts(linear int64, tb Timebase) (day int64, hour, minute, second, frame int) {
	dayFrames := tb.FramesPerDay()
	day = linear / dayFrames
	within := linear % dayFrames
	if within < 0 {
		day--
		within += dayFrames
	}

	base := int64(tb.TimecodeBase)
	if !tb.DropFrame {
		hour = int(within / (3600 * base))
		within %= 3600 * base
		minute = int(within / (60 * base))
		within %= 60 * base
		second = int(within / base)
		frame = int(within % base)
		return day, hour, minute, second, frame
	}

	drop := dropFrameIncrement(tb.TimecodeBase)
	minuteIndex := sort.Search(24*60, func(candidate int) bool {
		return within < dropFrameMinuteStart(candidate+1, base, drop)
	})
	if minuteIndex < 24*60 {
		start := dropFrameMinuteStart(minuteIndex, base, drop)
		if within >= start {
			hour = minuteIndex / 60
			minute = minuteIndex % 60
			labelFrame := int(within - start)
			if minuteIndex%10 != 0 {
				labelFrame += drop
			}
			second = labelFrame / tb.TimecodeBase
			frame = labelFrame % tb.TimecodeBase
			return day, hour, minute, second, frame
		}
	}

	return day, 0, 0, 0, 0
}

func dropFrameMinuteStart(minuteIndex int, base int64, drop int) int64 {
	m := int64(minuteIndex)
	droppedGroups := m - m/10
	start := m*60*base - int64(drop)*droppedGroups
	if minuteIndex%10 != 0 {
		start += int64(drop)
	}
	return start
}

// FormatTimecode formats a linear frame index. Multi-day values wrap at
// midnight; use TimecodeParts when the day offset matters.
func FormatTimecode(linear int64, tb Timebase) string {
	_, hour, minute, second, frame := TimecodeParts(linear, tb)
	frameSeparator := ":"
	if tb.DropFrame {
		frameSeparator = ";"
	}
	return fmt.Sprintf("%02d:%02d:%02d%s%02d", hour, minute, second, frameSeparator, frame)
}
