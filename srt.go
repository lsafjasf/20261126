package safereflow

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

var srtTimeRe = regexp.MustCompile(
	`(\d{1,2}):(\d{2}):(\d{2})[,.](\d{1,3})\s*-->\s*(\d{1,2}):(\d{2}):(\d{2})[,.](\d{1,3})`)

// ParseSRT reads SubRip subtitles into a CueDocument. SRT carries no canvas
// or geometry, so canvas and default style must be supplied by the caller.
// Cue boxes are left nil and resolved through the Measurer.
func ParseSRT(r io.Reader, canvas Canvas, style Style) (*CueDocument, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	var blocks [][]string
	var cur []string
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		line = strings.TrimPrefix(line, "\uFEFF")
		if strings.TrimSpace(line) == "" {
			if len(cur) > 0 {
				blocks = append(blocks, cur)
				cur = nil
			}
			continue
		}
		cur = append(cur, line)
	}
	if len(cur) > 0 {
		blocks = append(blocks, cur)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}

	doc := &CueDocument{Canvas: canvas, Style: style}
	for bi, block := range blocks {
		timeIdx := -1
		var m []string
		for i, ln := range block {
			if mm := srtTimeRe.FindStringSubmatch(ln); mm != nil {
				timeIdx = i
				m = mm
				break
			}
		}
		if timeIdx < 0 {
			return nil, fmt.Errorf("safereflow: srt block %d missing time line", bi+1)
		}
		start := parseSRTTime(m[1:5])
		end := parseSRTTime(m[5:9])
		text := strings.Join(block[timeIdx+1:], "\n")
		text = strings.TrimSpace(text)
		id := strings.TrimSpace(block[0])
		if timeIdx == 0 || id == "" || !isIntegerLine(id) {
			id = fmt.Sprintf("cue-%d", bi+1)
		}
		doc.Cues = append(doc.Cues, &Cue{
			ID:        id,
			TimeRange: TimeRange{Start: start, End: end},
			Text:      text,
		})
	}
	return doc, nil
}

func isIntegerLine(s string) bool {
	if s == "" {
		return false
	}
	_, err := strconv.Atoi(strings.TrimSpace(s))
	return err == nil
}

func parseSRTTime(parts []string) float64 {
	h, _ := strconv.Atoi(parts[0])
	min, _ := strconv.Atoi(parts[1])
	sec, _ := strconv.Atoi(parts[2])
	ms := parts[3]
	for len(ms) < 3 {
		ms += "0"
	}
	msec, _ := strconv.Atoi(ms[:3])
	return float64(h)*3600 + float64(min)*60 + float64(sec) + float64(msec)/1000
}

// FormatSRTTime formats seconds as HH:MM:SS,mmm.
func FormatSRTTime(t float64) string {
	if t < 0 {
		t = 0
	}
	totalMS := int64((t * 1000) + 0.5)
	ms := totalMS % 1000
	totalSec := totalMS / 1000
	s := totalSec % 60
	totalMin := totalSec / 60
	m := totalMin % 60
	h := totalMin / 60
	return fmt.Sprintf("%02d:%02d:%02d,%03d", h, m, s, ms)
}

// WriteSRT writes the document cues in SubRip format. Geometry/migration is
// lossy in SRT; use WriteJSON for full-fidelity edit lists.
func WriteSRT(w io.Writer, doc *CueDocument) error {
	for i, c := range doc.Cues {
		if _, err := fmt.Fprintf(w, "%d\n", i+1); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "%s --> %s\n", FormatSRTTime(c.Start), FormatSRTTime(c.End)); err != nil {
			return err
		}
		text := c.Text
		if strings.TrimSpace(text) == "" {
			text = " "
		}
		if _, err := fmt.Fprint(w, text, "\n\n"); err != nil {
			return err
		}
	}
	return nil
}
