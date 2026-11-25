package vttlayout

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Parse reads a WebVTT document. It is lenient about CRLF, comma decimals and
// SRT style blocks, but returns an error when the input has no WEBVTT header
// or a cue timing line cannot be parsed.
func Parse(input string) (*Document, error) {
	input = strings.ReplaceAll(input, "\r\n", "\n")
	input = strings.ReplaceAll(input, "\r", "\n")
	lines := strings.Split(input, "\n")

	doc := &Document{Regions: map[string]*Region{}}

	idx := 0
	// Locate the WEBVTT signature (it must be the first non empty line).
	for idx < len(lines) && strings.TrimSpace(lines[idx]) == "" {
		idx++
	}
	if idx >= len(lines) || !strings.HasPrefix(strings.TrimSpace(lines[idx]), "WEBVTT") {
		return nil, fmt.Errorf("not a WebVTT file: missing WEBVTT signature")
	}
	idx++

	cueOrder := 0
	for idx < len(lines) {
		line := lines[idx]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			idx++
			continue
		}

		// NOTE blocks: ignore until blank line. NOTE must be the whole
		// first token (a cue id like "NOTEX" is not a note).
		if trimmed == "NOTE" || strings.HasPrefix(trimmed, "NOTE ") ||
			strings.HasPrefix(trimmed, "NOTE\t") {
			for idx < len(lines) && strings.TrimSpace(lines[idx]) != "" {
				idx++
			}
			continue
		}
		// STYLE / REGION header companion blocks are handled below; any other
		// uppercase block marker we do not model is skipped to its end.
		if strings.HasPrefix(trimmed, "STYLE") {
			for idx < len(lines) && strings.TrimSpace(lines[idx]) != "" {
				idx++
			}
			continue
		}

		if trimmed == "REGION" || strings.HasPrefix(trimmed, "REGION ") ||
			strings.HasPrefix(trimmed, "Region:") {
			next, w := parseRegionBlock(lines, idx, doc)
			doc.Warnings = append(doc.Warnings, w...)
			idx = next
			continue
		}

		// Cue block: optional identifier line, then the timing line.
		idLine := ""
		if !strings.Contains(trimmed, "-->") {
			// Could be a cue identifier. Consume it and expect timing next.
			idLine = trimmed
			idx++
			if idx >= len(lines) {
				return nil, fmt.Errorf("line %d: cue identifier %q without timing", idx, idLine)
			}
			trimmed = strings.TrimSpace(lines[idx])
		}
		if !strings.Contains(trimmed, "-->") {
			// Unknown block; skip defensively.
			doc.Warnings = append(doc.Warnings, ParseWarning{Line: idx + 1, Msg: "unrecognized block, skipped: " + trimmed})
			for idx < len(lines) && strings.TrimSpace(lines[idx]) != "" {
				idx++
			}
			continue
		}

		cue, err := parseTimingLine(trimmed)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", idx+1, err)
		}
		cue.ID = idLine
		cue.Order = cueOrder
		cueOrder++
		idx++

		var payload []string
		for idx < len(lines) && strings.TrimSpace(lines[idx]) != "" {
			payload = append(payload, lines[idx])
			idx++
		}
		cue.Text = DecodeText(strings.Join(payload, "\n"))
		doc.Cues = append(doc.Cues, cue)
	}

	// Resolve/validate region references.
	for _, c := range doc.Cues {
		if c.Settings.Region != "" {
			if _, ok := doc.Regions[c.Settings.Region]; !ok {
				doc.Warnings = append(doc.Warnings, ParseWarning{
					Msg: fmt.Sprintf("cue #%d references missing region %q", c.Order+1, c.Settings.Region),
				})
			}
		}
	}
	return doc, nil
}

func parseRegionBlock(lines []string, start int, doc *Document) (int, []ParseWarning) {
	var warnings []ParseWarning
	r := &Region{
		WidthPct:        100,
		Lines:           3,
		RegionAnchorX:   0,
		RegionAnchorY:   100,
		ViewportAnchorX: 0,
		ViewportAnchorY: 100,
	}
	idx := start + 1
	// Settings may also trail the REGION marker, whitespace separated.
	headerFields := strings.Fields(strings.TrimPrefix(strings.TrimSpace(lines[start]), "REGION"))
	applyRegionFields(r, headerFields, &warnings, start+1)

	for idx < len(lines) {
		raw := strings.TrimSpace(lines[idx])
		if raw == "" {
			idx++
			break
		}
		if !strings.Contains(raw, ":") {
			warnings = append(warnings, ParseWarning{Line: idx + 1, Msg: "bad region setting: " + raw})
			idx++
			continue
		}
		applyRegionFields(r, []string{raw}, &warnings, idx+1)
		idx++
	}
	if r.ID == "" {
		r.ID = fmt.Sprintf("region%d", len(doc.Regions)+1)
		warnings = append(warnings, ParseWarning{Line: start + 1, Msg: "REGION block without id, assigned " + r.ID})
	}
	if _, exists := doc.Regions[r.ID]; !exists {
		doc.RegionOrder = append(doc.RegionOrder, r.ID)
	}
	doc.Regions[r.ID] = r
	return idx, warnings
}

func applyRegionFields(r *Region, fields []string, warnings *[]ParseWarning, lineNo int) {
	for _, field := range fields {
		key, val, ok := cutSetting(field)
		if !ok {
			continue
		}
		switch key {
		case "id":
			r.ID = val
		case "width":
			if pct, err := parsePct(val); err == nil {
				r.WidthPct = pct
			}
		case "lines":
			if n, err := strconv.Atoi(val); err == nil && n > 0 {
				r.Lines = n
			}
		case "regionanchor":
			if x, y, err := parseAnchor(val); err == nil {
				r.RegionAnchorX, r.RegionAnchorY = x, y
			}
		case "viewportanchor":
			if x, y, err := parseAnchor(val); err == nil {
				r.ViewportAnchorX, r.ViewportAnchorY = x, y
			}
		case "scroll":
			r.Scroll = val
		case "priority", "x-priority":
			if n, err := strconv.Atoi(val); err == nil {
				r.Priority = n
				r.PrioritySet = true
			}
		default:
			// Unknown/extension settings are intentionally ignored.
		}
	}
}

func parseTimingLine(line string) (*Cue, error) {
	before, after, ok := strings.Cut(line, "-->")
	if !ok {
		return nil, fmt.Errorf("expected timing line, got %q", line)
	}
	start, err := ParseTimestamp(strings.TrimSpace(before))
	if err != nil {
		return nil, err
	}
	afterFields := strings.Fields(after)
	if len(afterFields) == 0 {
		return nil, fmt.Errorf("missing end timestamp in %q", line)
	}
	end, err := ParseTimestamp(afterFields[0])
	if err != nil {
		return nil, err
	}
	c := &Cue{StartMS: start, EndMS: end}
	settings := strings.Join(afterFields[1:], " ")
	c.RawSettings = settings
	c.Settings = parseCueSettings(afterFields[1:])
	return c, nil
}

func parseCueSettings(tokens []string) CueSettings {
	s := CueSettings{Align: "center", SnapToLines: true, LineAlign: "start"}
	for _, tok := range tokens {
		key, val, ok := cutSetting(tok)
		if !ok {
			continue
		}
		// Values such as "5%,start" carry an alignment suffix.
		primary, suffix, _ := strings.Cut(val, ",")
		switch key {
		case "region":
			s.Region = val
		case "vertical":
			s.Vertical = val
		case "align":
			s.Align = val
		case "line":
			if strings.HasSuffix(primary, "%") {
				if pct, err := parsePct(primary); err == nil {
					s.HasLinePct = true
					s.LinePct = pct
					s.SnapToLines = false
				}
			} else if n, err := strconv.ParseFloat(primary, 64); err == nil {
				s.HasLineNumber = true
				s.LineNumber = n
			}
			if suffix != "" {
				s.LineAlign = suffix
			}
		case "position":
			if pct, err := parsePct(primary); err == nil {
				s.HasPosition = true
				s.PositionPct = pct
			}
			if suffix != "" {
				s.PositionAlign = suffix
			}
		case "size":
			if pct, err := parsePct(primary); err == nil {
				s.HasSize = true
				s.SizePct = pct
			}
		case "priority", "x-priority":
			if n, err := strconv.Atoi(primary); err == nil {
				s.Priority = n
				s.PrioritySet = true
			}
		}
	}
	return s
}

func cutSetting(field string) (key, val string, ok bool) {
	k, v, found := strings.Cut(field, ":")
	if !found {
		return "", "", false
	}
	return strings.TrimSpace(k), strings.TrimSpace(v), true
}

func parsePct(s string) (float64, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "%")
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	if math.IsNaN(n) {
		return 0, fmt.Errorf("NaN percentage")
	}
	return n, nil
}

func parseAnchor(s string) (x, y float64, err error) {
	parts := strings.Split(s, ",")
	if len(parts) != 2 {
		// Also accept whitespace separated "0% 100%".
		parts = strings.Fields(s)
	}
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("bad anchor %q", s)
	}
	x, err = parsePct(parts[0])
	if err != nil {
		return 0, 0, err
	}
	y, err = parsePct(parts[1])
	if err != nil {
		return 0, 0, err
	}
	return x, y, nil
}

// DecodeText removes inline WebVTT tags and decodes character entities.
func DecodeText(s string) string {
	var b strings.Builder
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == '<' {
			j := i + 1
			for j < len(runes) && runes[j] != '>' {
				j++
			}
			if j < len(runes) {
				i = j
				continue
			}
		}
		if r == '&' {
			j := i + 1
			for j < len(runes) && runes[j] != ';' && j-i <= 12 {
				j++
			}
			if j < len(runes) && runes[j] == ';' {
				if dec, ok := decodeEntity(string(runes[i+1 : j])); ok {
					b.WriteString(dec)
					i = j
					continue
				}
			}
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

func decodeEntity(e string) (string, bool) {
	switch e {
	case "amp":
		return "&", true
	case "lt":
		return "<", true
	case "gt":
		return ">", true
	case "quot":
		return `"`, true
	case "apos":
		return "'", true
	case "nbsp":
		return "\u00a0", true
	}
	if strings.HasPrefix(e, "#x") || strings.HasPrefix(e, "#X") {
		if n, err := strconv.ParseInt(e[2:], 16, 32); err == nil && n > 0 {
			return string(rune(n)), true
		}
		return "", false
	}
	if strings.HasPrefix(e, "#") {
		if n, err := strconv.Atoi(e[1:]); err == nil && n > 0 {
			return string(rune(n)), true
		}
	}
	return "", false
}
