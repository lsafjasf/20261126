package vtt

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseTimestamp parses a WebVTT (or lenient SRT-style) timestamp and
// returns its value in milliseconds.
//
// Accepted shapes:
//
//	mm:ss.ttt
//	hh:mm:ss.ttt
//
// The decimal separator may be '.' or ',' for tolerant input.
func ParseTimestamp(value string) (int64, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, fmt.Errorf("empty timestamp")
	}
	normalized := strings.ReplaceAll(trimmed, ",", ".")
	parts := strings.Split(normalized, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, fmt.Errorf("invalid timestamp %q", value)
	}

	last := strings.SplitN(parts[len(parts)-1], ".", 2)
	secondsText := last[0]
	var fractionText string
	if len(last) == 2 {
		fractionText = last[1]
	}

	seconds, err := strconv.ParseInt(secondsText, 10, 64)
	if err != nil || seconds < 0 || seconds >= 60 {
		return 0, fmt.Errorf("invalid seconds in timestamp %q", value)
	}

	millis := seconds * 1000
	if fractionText != "" {
		if len(fractionText) > 3 {
			fractionText = fractionText[:3]
		}
		fraction, err := strconv.ParseInt(fractionText, 10, 64)
		if err != nil || fraction < 0 {
			return 0, fmt.Errorf("invalid fraction in timestamp %q", value)
		}
		for pad := len(fractionText); pad < 3; pad++ {
			fraction *= 10
		}
		millis += fraction
	}

	minutes, err := parseTwoDigits(parts[len(parts)-2], value)
	if err != nil {
		return 0, err
	}
	millis += minutes * 60 * 1000

	if len(parts) == 3 {
		hours, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil || hours < 0 {
			return 0, fmt.Errorf("invalid hours in timestamp %q", value)
		}
		millis += hours * 3600 * 1000
	}

	return millis, nil
}

func parseTwoDigits(text, original string) (int64, error) {
	number, err := strconv.ParseInt(text, 10, 64)
	if err != nil || number < 0 || number >= 60 {
		return 0, fmt.Errorf("invalid component in timestamp %q", original)
	}
	return number, nil
}

// FormatTimestamp renders milliseconds as a canonical WebVTT timestamp.
// Sub-hour values use "mm:ss.ttt"; anything from one hour upward uses
// "hh:mm:ss.ttt" with a zero-padded two-digit hour field.
func FormatTimestamp(millis int64) string {
	if millis < 0 {
		millis = 0
	}
	hours := millis / 3_600_000
	minutes := (millis % 3_600_000) / 60_000
	seconds := (millis % 60_000) / 1000
	fraction := millis % 1000
	if hours > 0 {
		return fmt.Sprintf("%02d:%02d:%02d.%03d", hours, minutes, seconds, fraction)
	}
	return fmt.Sprintf("%02d:%02d.%03d", minutes, seconds, fraction)
}
