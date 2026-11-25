package opusalign

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

var srtTimePattern = regexp.MustCompile(`^(\d{2,}):(\d{2}):(\d{2})[,.](\d{1,3})$`)

func ParseSRTTime(value string) (int64, error) {
	matches := srtTimePattern.FindStringSubmatch(strings.TrimSpace(value))
	if matches == nil {
		return 0, fmt.Errorf("invalid SRT timestamp %q", value)
	}
	hours, _ := strconv.ParseInt(matches[1], 10, 64)
	minutes, _ := strconv.ParseInt(matches[2], 10, 64)
	seconds, _ := strconv.ParseInt(matches[3], 10, 64)
	millisText := matches[4]
	millis, _ := strconv.ParseInt(millisText, 10, 64)
	switch len(millisText) {
	case 1:
		millis *= 100
	case 2:
		millis *= 10
	}
	totalMS := hours*3_600_000 + minutes*60_000 + seconds*1000 + millis
	return totalMS * 48, nil
}

func ParseSRT(reader io.Reader) ([]Cue, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	blocks := make([][]string, 0)
	current := make([]string, 0)
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if strings.TrimSpace(line) == "" {
			if len(current) > 0 {
				blocks = append(blocks, current)
				current = nil
			}
			continue
		}
		current = append(current, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(current) > 0 {
		blocks = append(blocks, current)
	}

	cues := make([]Cue, 0, len(blocks))
	for blockIndex, lines := range blocks {
		timeLineIndex := -1
		for i, line := range lines {
			if strings.Contains(line, "-->") {
				timeLineIndex = i
				break
			}
		}
		if timeLineIndex < 0 {
			return nil, fmt.Errorf("SRT block %d lacks time range", blockIndex+1)
		}
		parts := strings.SplitN(lines[timeLineIndex], "-->", 2)
		start, err := ParseSRTTime(parts[0])
		if err != nil {
			return nil, err
		}
		end, err := ParseSRTTime(parts[1])
		if err != nil {
			return nil, err
		}
		id := strconv.Itoa(blockIndex + 1)
		if timeLineIndex > 0 && strings.TrimSpace(lines[0]) != "" {
			id = strings.TrimSpace(lines[0])
		}
		textLines := lines[timeLineIndex+1:]
		cues = append(cues, Cue{
			ID:          id,
			StartSample: start,
			EndSample:   end,
			Text:        strings.Join(textLines, "\n"),
		})
	}
	return cues, nil
}

func WriteSRT(writer io.Writer, cues []SyncedCue) error {
	sequence := 1
	for _, cue := range cues {
		if cue.Text == "" {
			continue
		}
		start := FormatSRTTime(cue.Range.Start)
		end := FormatSRTTime(cue.Range.End)
		if _, err := fmt.Fprintf(writer, "%d\n%s --> %s\n%s\n\n", sequence, start, end, cue.Text); err != nil {
			return err
		}
		sequence++
	}
	return nil
}
