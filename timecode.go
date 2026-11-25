package timeline

import (
	"fmt"
	"strconv"
	"strings"
)

// FrameToTimecode converts an absolute frame number to SMPTE timecode. NTSC
// rates use drop-frame notation (a semicolon before the frame field).
func FrameToTimecode(rate Rate, frame int64) (string, error) {
	if err := rate.Validate(); err != nil {
		return "", err
	}
	if frame < 0 || frame >= rate.framesPerDay() {
		return "", fmt.Errorf("frame %d outside 24-hour timecode range", frame)
	}

	drop := rate.dropFrames()
	if drop == 0 {
		return formatNominal(rate.nominalFPS(), frame, ':'), nil
	}

	framesPer10Minutes := rate.framesPer10Minutes()
	framesPerMinute := rate.framesPerMinute()
	tenMinuteGroups := frame / framesPer10Minutes
	remainder := frame % framesPer10Minutes

	// Re-insert the frame numbers omitted by drop-frame timecode. This is the
	// canonical SMPTE conversion; in particular, the tenth minute of each
	// group has no dropped frame numbers.
	if remainder > drop {
		frame += drop*9*tenMinuteGroups + drop*((remainder-drop)/framesPerMinute)
	} else {
		frame += drop * 9 * tenMinuteGroups
	}
	return formatNominal(rate.nominalFPS(), frame, ';'), nil
}

func formatNominal(fps, frame int64, separator byte) string {
	ff := frame % fps
	frame /= fps
	ss := frame % 60
	frame /= 60
	mm := frame % 60
	hh := frame / 60
	return fmt.Sprintf("%02d:%02d:%02d%c%02d", hh, mm, ss, separator, ff)
}

// ParseTimecode converts SMPTE timecode to an absolute frame number.
func ParseTimecode(rate Rate, tc string) (int64, error) {
	if err := rate.Validate(); err != nil {
		return 0, err
	}
	tc = strings.TrimSpace(tc)
	if tc == "" {
		return 0, fmt.Errorf("empty timecode")
	}

	dropNotation := strings.ContainsRune(tc, ';')
	fields := strings.FieldsFunc(tc, func(r rune) bool { return r == ':' || r == ';' })
	if len(fields) != 4 {
		return 0, fmt.Errorf("invalid timecode %q: want HH:MM:SS:FF", tc)
	}

	var values [4]int64
	for i, field := range fields {
		if field == "" {
			return 0, fmt.Errorf("invalid timecode %q: empty field", tc)
		}
		v, err := strconv.ParseInt(field, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid timecode %q: %w", tc, err)
		}
		values[i] = v
	}
	hh, mm, ss, ff := values[0], values[1], values[2], values[3]

	fps := rate.nominalFPS()
	if hh < 0 || hh >= 24 {
		return 0, fmt.Errorf("invalid timecode %q: hours out of range", tc)
	}
	if mm < 0 || mm >= 60 {
		return 0, fmt.Errorf("invalid timecode %q: minutes out of range", tc)
	}
	if ss < 0 || ss >= 60 {
		return 0, fmt.Errorf("invalid timecode %q: seconds out of range", tc)
	}
	if ff < 0 || ff >= fps {
		return 0, fmt.Errorf("invalid timecode %q: frames out of range for %d fps", tc, fps)
	}

	drop := rate.dropFrames()
	if drop == 0 {
		if dropNotation {
			return 0, fmt.Errorf("timecode %q uses drop-frame notation at a non-drop rate", tc)
		}
		return ((hh*3600+mm*60+ss)*fps + ff), nil
	}
	if !dropNotation {
		return 0, fmt.Errorf("timecode %q must use drop-frame notation at %s", tc, rate)
	}
	if mm%10 != 0 && ss == 0 && ff < drop {
		return 0, fmt.Errorf("timecode %q refers to a dropped frame number", tc)
	}

	nominal := (hh*3600+mm*60+ss)*fps + ff
	totalMinutes := hh*60 + mm
	return nominal - drop*(totalMinutes-totalMinutes/10), nil
}
