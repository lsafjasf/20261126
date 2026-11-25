package vtt

import (
	"fmt"
	"strings"
)

const bom = "\ufeff"

// Parse decodes a WebVTT document. It accepts a UTF-8 BOM and CRLF/CR
// line endings. STYLE, REGION and NOTE blocks are preserved so the file
// can be re-serialized without losing metadata.
func Parse(data []byte) (*Document, error) {
	return ParseString(string(data))
}

// ParseString decodes a WebVTT document from text.
func ParseString(input string) (*Document, error) {
	text := strings.TrimPrefix(input, bom)
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")

	blocks := splitBlocks(text)
	if len(blocks) == 0 {
		return nil, fmt.Errorf("not a WebVTT file: missing WEBVTT signature")
	}

	document := &Document{}
	header, ok := parseHeader(blocks[0])
	if !ok {
		return nil, fmt.Errorf("not a WebVTT file: first block must start with WEBVTT")
	}
	document.Header = header

	for index, raw := range blocks[1:] {
		block, err := parseBlock(raw)
		if err != nil {
			return nil, fmt.Errorf("block %d: %w", index+1, err)
		}
		document.Blocks = append(document.Blocks, block)
	}
	return document, nil
}

func splitBlocks(text string) [][]string {
	lines := strings.Split(text, "\n")
	var blocks [][]string
	var current []string
	flush := func() {
		if len(current) > 0 {
			blocks = append(blocks, current)
			current = nil
		}
	}
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		current = append(current, line)
	}
	flush()
	return blocks
}

func parseHeader(lines []string) (string, bool) {
	first := lines[0]
	if first != "WEBVTT" && !strings.HasPrefix(first, "WEBVTT ") && !strings.HasPrefix(first, "WEBVTT\t") {
		return "", false
	}
	return strings.TrimSpace(strings.TrimPrefix(first, "WEBVTT")), true
}

func parseBlock(lines []string) (Block, error) {
	first := strings.TrimSpace(lines[0])
	switch {
	case first == "NOTE" || strings.HasPrefix(first, "NOTE ") || strings.HasPrefix(first, "NOTE\t"):
		return &Note{Text: strings.Join(lines, "\n")}, nil
	case first == "STYLE":
		return &Style{Text: strings.Join(lines[1:], "\n")}, nil
	case first == "REGION":
		return &Region{Text: strings.Join(lines[1:], "\n")}, nil
	default:
		return parseCue(lines)
	}
}

func parseCue(lines []string) (*Cue, error) {
	timingIndex := 0
	cue := &Cue{}

	if !strings.Contains(lines[0], "-->") {
		cue.ID = lines[0]
		timingIndex = 1
	}
	if timingIndex >= len(lines) {
		return nil, fmt.Errorf("cue %q has no timing line", cue.ID)
	}

	timingLine := lines[timingIndex]
	fields := strings.Fields(timingLine)
	if len(fields) < 3 || fields[1] != "-->" {
		return nil, fmt.Errorf("cue %q has malformed timing line %q", cue.ID, timingLine)
	}

	start, err := ParseTimestamp(fields[0])
	if err != nil {
		return nil, fmt.Errorf("cue %q: %w", cue.ID, err)
	}
	end, err := ParseTimestamp(fields[2])
	if err != nil {
		return nil, fmt.Errorf("cue %q: %w", cue.ID, err)
	}
	if end < start {
		return nil, fmt.Errorf("cue %q ends (%d) before it starts (%d)", cue.ID, end, start)
	}
	cue.Start = start
	cue.End = end
	cue.Settings = strings.Join(fields[3:], " ")
	cue.Text = strings.Join(lines[timingIndex+1:], "\n")
	if strings.TrimSpace(cue.Text) == "" {
		return nil, fmt.Errorf("cue %q has no payload text", cue.ID)
	}
	return cue, nil
}
