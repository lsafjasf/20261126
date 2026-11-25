package cuemap

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ParseSRT parses a small, deterministic subset of SubRip cue blocks.
// Ruby markup is then handled by ParseCue. Every returned cue preserves its
// complete original block in Raw, making fallback references traceable.
func ParseSRT(input string) ([]Cue, error) {
	input = strings.ReplaceAll(input, "\r\n", "\n")
	input = strings.ReplaceAll(input, "\r", "\n")
	blocks := splitSRTBlocks(input)
	cues := make([]Cue, 0, len(blocks))
	var errs []error

	for blockIndex, block := range blocks {
		lines := strings.Split(block, "\n")
		if len(lines) < 2 {
			errs = append(errs, fmt.Errorf("%d: short cue block", blockIndex+1))
			continue
		}

		lineIndex := 0
		sequence := blockIndex + 1
		if n, err := strconv.Atoi(strings.TrimSpace(lines[0])); err == nil {
			sequence = n
			lineIndex++
		}
		if lineIndex >= len(lines) || !strings.Contains(lines[lineIndex], "-->") {
			errs = append(errs, fmt.Errorf("%d: missing time line", sequence))
			continue
		}

		startMS, endMS, err := parseSRTTiming(lines[lineIndex])
		if err != nil {
			errs = append(errs, fmt.Errorf("%d: %w", sequence, err))
			continue
		}
		lineIndex++

		markup := strings.Join(lines[lineIndex:], "\n")
		cue := ParseCue(strconv.Itoa(sequence), markup)
		cue.Index = sequence
		cue.StartMS = startMS
		cue.EndMS = endMS
		cue.Raw = strings.TrimSpace(block)
		cues = append(cues, cue)
	}
	return cues, errors.Join(errs...)
}

func splitSRTBlocks(input string) []string {
	rawBlocks := strings.Split(input, "\n\n")
	blocks := make([]string, 0, len(rawBlocks))
	for _, block := range rawBlocks {
		block = strings.TrimSpace(block)
		if block != "" {
			blocks = append(blocks, block)
		}
	}
	return blocks
}

func parseSRTTiming(line string) (int64, int64, error) {
	parts := strings.SplitN(line, "-->", 2)
	if len(parts) != 2 {
		return 0, 0, errors.New("invalid timing line")
	}
	startFields := strings.Fields(parts[0])
	if len(startFields) == 0 {
		return 0, 0, errors.New("missing start time")
	}
	start, err := parseSRTTime(startFields[0])
	if err != nil {
		return 0, 0, err
	}
	endFields := strings.Fields(parts[1])
	if len(endFields) == 0 {
		return 0, 0, errors.New("missing end time")
	}
	end, err := parseSRTTime(endFields[0])
	if err != nil {
		return 0, 0, err
	}
	return start, end, nil
}

func parseSRTTime(value string) (int64, error) {
	comma := strings.Split(value, ",")
	if len(comma) != 2 {
		return 0, errors.New("bad fractional time: " + value)
	}
	hms := strings.Split(comma[0], ":")
	if len(hms) != 3 {
		return 0, errors.New("bad clock time: " + value)
	}
	hours, err := strconv.Atoi(hms[0])
	if err != nil {
		return 0, err
	}
	minutes, err := strconv.Atoi(hms[1])
	if err != nil {
		return 0, err
	}
	seconds, err := strconv.Atoi(hms[2])
	if err != nil {
		return 0, err
	}
	millis, err := strconv.Atoi(comma[1])
	if err != nil {
		return 0, err
	}
	return int64(hours)*3_600_000 + int64(minutes)*60_000 +
		int64(seconds)*1000 + int64(millis), nil
}
