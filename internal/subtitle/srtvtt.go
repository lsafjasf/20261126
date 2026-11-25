package subtitle

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	arrowSRT = regexp.MustCompile(`(\d{1,2}:\d{2}:\d{2}[.,]\d{1,3})\s*-->\s*(\d{1,2}:\d{2}:\d{2}[.,]\d{1,3})`)
	arrowVTT = regexp.MustCompile(`(?:\d{1,2}:)?\d{2}:\d{2}[.,]\d{1,3}\s*-->\s*(?:\d{1,2}:)?\d{2}:\d{2}[.,]\d{1,3}`)
	tag      = regexp.MustCompile(`<[^>]+>`)
	cueStyle = regexp.MustCompile(`(?i)<c\.[^>]+>`)
)

// parseSRTVTT handles both WebVTT and SubRip; their block structure is the
// same, differing only in cue numbering, timestamp hour optionality and
// NOTE/REGION blocks.
func parseSRTVTT(text string, format Format) (*File, error) {
	f := &File{Format: format}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	var (
		cue       Cue
		body      []string
		inCue     bool
		skipBlock bool
	)

	flush := func() {
		if !inCue {
			return
		}
		cue.RawText = strings.Join(body, "\n")
		cue.Visible = stripInline(cue.RawText)
		cue.Fonts = inlineFonts(cue.Index, cue.RawText)
		f.Cues = append(f.Cues, cue)
		body = nil
		cue = Cue{}
		inCue = false
	}

	for _, ln := range lines {
		trimmed := strings.TrimSpace(ln)
		if trimmed == "" {
			flush()
			skipBlock = false
			continue
		}
		if skipBlock {
			continue
		}
		if strings.HasPrefix(trimmed, "NOTE") || strings.HasPrefix(trimmed, "REGION") ||
			strings.HasPrefix(trimmed, "STYLE") || strings.HasPrefix(trimmed, "WEBVTT") {
			skipBlock = true
			continue
		}
		if m := arrowSRT.FindStringSubmatch(trimmed); m != nil {
			start, err1 := parseColonTime(m[1])
			end, err2 := parseColonTime(m[2])
			if err1 != nil || err2 != nil {
				f.Warnings = append(f.Warnings, "unparseable timing line: "+trimmed)
				continue
			}
			cue.Index = len(f.Cues) + 1
			cue.Start, cue.End = start, end
			inCue = true
			continue
		}
		if format == FormatVTT && arrowVTT.MatchString(trimmed) {
			parts := strings.SplitN(trimmed, "-->", 2)
			start, err1 := parseVTTTime(strings.TrimSpace(parts[0]))
			end, err2 := parseVTTTime(strings.Fields(parts[1])[0])
			if err1 != nil || err2 != nil {
				f.Warnings = append(f.Warnings, "unparseable timing line: "+trimmed)
				continue
			}
			cue.Index = len(f.Cues) + 1
			cue.Start, cue.End = start, end
			inCue = true
			continue
		}
		if inCue {
			body = append(body, ln)
			continue
		}
		// Standalone numeric line is the optional SRT cue number; ignore.
		if _, err := strconv.Atoi(trimmed); err != nil {
			f.Warnings = append(f.Warnings, "line outside cue ignored: "+trimmed)
		}
	}
	flush()
	return f, nil
}

func parseColonTime(s string) (time.Duration, error) {
	s = strings.Replace(s, ",", ".", 1)
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return 0, strconv.ErrSyntax
	}
	h, e1 := strconv.Atoi(parts[0])
	m, e2 := strconv.Atoi(parts[1])
	sec, e3 := strconv.ParseFloat(parts[2], 64)
	if e1 != nil || e2 != nil || e3 != nil {
		return 0, strconv.ErrSyntax
	}
	return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute +
		time.Duration(sec*float64(time.Second)), nil
}

func parseVTTTime(s string) (time.Duration, error) {
	s = strings.Replace(s, ",", ".", 1)
	parts := strings.Split(s, ":")
	switch len(parts) {
	case 2:
		m, e1 := strconv.Atoi(parts[0])
		sec, e2 := strconv.ParseFloat(parts[1], 64)
		if e1 != nil || e2 != nil {
			return 0, strconv.ErrSyntax
		}
		return time.Duration(m)*time.Minute + time.Duration(sec*float64(time.Second)), nil
	case 3:
		return parseColonTime(s)
	default:
		return 0, strconv.ErrSyntax
	}
}

// stripInline removes HTML-style tags but keeps every text rune, so the
// coverage check sees exactly what the renderer shapes.
func stripInline(s string) string {
	s = cueStyle.ReplaceAllString(s, "")
	s = tag.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

var fontTag = regexp.MustCompile(`(?i)<font\s+[^>]*?face=["']?([^"'>]+)["']?[^>]*>`)

func inlineFonts(cueIndex int, raw string) []FontRef {
	matches := fontTag.FindAllStringSubmatch(raw, -1)
	if len(matches) == 0 {
		return nil
	}
	out := make([]FontRef, 0, len(matches))
	for _, m := range matches {
		out = append(out, FontRef{
			Name:   strings.TrimSpace(m[1]),
			Source: "inline:cue:" + strconv.Itoa(cueIndex),
		})
	}
	return out
}
