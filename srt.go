package sdh

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// ParseSRT reads SubRip data and returns its cues in file order.
//
// The parser tolerates UTF-8 BOM, CRLF/CR/LF line endings, missing sequence
// numbers and trailing SRT positioning directives after the timing line.
func ParseSRT(r io.Reader) ([]Cue, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return ParseSRTBytes(data)
}

// ParseSRTBytes is the byte-slice form of ParseSRT.
func ParseSRTBytes(data []byte) ([]Cue, error) {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")

	blocks := splitBlocks(text)
	cues := make([]Cue, 0, len(blocks))
	autoIndex := 0
	for _, block := range blocks {
		lines := strings.Split(block, "\n")
		// Drop leading/trailing blank lines inside a block.
		for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
			lines = lines[1:]
		}
		for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
			lines = lines[:len(lines)-1]
		}
		if len(lines) == 0 {
			continue
		}

		timingIdx := -1
		for i, ln := range lines {
			if strings.Contains(ln, "-->") {
				timingIdx = i
				break
			}
		}
		if timingIdx == -1 {
			// Not a cue block: WEBVTT header, NOTE/REGION blocks or stray
			// metadata. Skip it rather than failing the whole file.
			continue
		}

		var id string
		var index int
		if timingIdx >= 1 {
			head := strings.TrimSpace(lines[timingIdx-1])
			if n, err := strconv.Atoi(head); err == nil {
				index = n
				id = head
			}
		}
		if id == "" {
			autoIndex++
			index = autoIndex
			id = strconv.Itoa(index)
		}

		start, end, err := parseTimingLine(lines[timingIdx])
		if err != nil {
			return nil, fmt.Errorf("sdh: srt cue %s: %w", id, err)
		}

		bodyLines := lines[timingIdx+1:]
		text := strings.Join(bodyLines, "\n")
		if strings.TrimSpace(text) == "" {
			// Cues without body text are kept: they still occupy the
			// timeline and callers decide whether to drop them.
			text = ""
		}

		cues = append(cues, Cue{
			ID:    id,
			Index: index,
			Start: start,
			End:   end,
			Text:  text,
		})
	}
	return cues, nil
}

func splitBlocks(text string) []string {
	var blocks []string
	var cur []string
	for _, ln := range strings.Split(text, "\n") {
		if strings.TrimSpace(ln) == "" {
			if len(cur) > 0 {
				blocks = append(blocks, strings.Join(cur, "\n"))
				cur = nil
			}
			continue
		}
		cur = append(cur, ln)
	}
	if len(cur) > 0 {
		blocks = append(blocks, strings.Join(cur, "\n"))
	}
	return blocks
}

func parseTimingLine(line string) (time.Duration, time.Duration, error) {
	parts := strings.SplitN(line, "-->", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("bad timing line %q", line)
	}
	start, err := ParseTimestamp(parts[0])
	if err != nil {
		return 0, 0, err
	}
	// The end side may carry positioning directives after the timestamp,
	// e.g. "00:00:02,000  X1:160 X2:480 Y1:200 Y2:260".
	endField := strings.TrimSpace(parts[1])
	if i := strings.IndexAny(endField, " \t"); i >= 0 {
		endField = endField[:i]
	}
	end, err := ParseTimestamp(endField)
	if err != nil {
		return 0, 0, err
	}
	if end < start {
		return 0, 0, fmt.Errorf("end %s before start %s", end, start)
	}
	return start, end, nil
}

// RenderSRT renders cues as SubRip text. Indexes are reassigned 1..n in
// slice order; cue intervals and text are preserved.
func RenderSRT(cues []Cue) []byte {
	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	_ = WriteSRT(w, cues)
	_ = w.Flush()
	return b.Bytes()
}

// WriteSRT streams the SRT rendering to w.
func WriteSRT(w io.Writer, cues []Cue) error {
	for i, cue := range cues {
		if i > 0 {
			if _, err := io.WriteString(w, "\n"); err != nil {
				return err
			}
		}
		idx := cue.Index
		if idx == 0 {
			idx = i + 1
		}
		if _, err := fmt.Fprintf(w, "%d\n%s --> %s\n",
			idx, FormatSRTTime(cue.Start), FormatSRTTime(cue.End)); err != nil {
			return err
		}
		if _, err := io.WriteString(w, cue.Text); err != nil {
			return err
		}
		if !strings.HasSuffix(cue.Text, "\n") {
			if _, err := io.WriteString(w, "\n"); err != nil {
				return err
			}
		}
	}
	return nil
}
