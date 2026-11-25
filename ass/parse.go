package ass

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

var defaultStyleFormat = []string{
	"name", "fontname", "fontsize", "primarycolour", "secondarycolour",
	"outlinecolour", "backcolour", "bold", "italic", "underline",
	"strikeout", "scalex", "scaley", "spacing", "angle", "borderstyle",
	"outline", "shadow", "alignment", "marginl", "marginr", "marginv",
	"encoding",
}

var defaultEventFormat = []string{
	"layer", "start", "end", "style", "name", "marginl", "marginr",
	"marginv", "effect", "text",
}

// ParseFile opens and parses an ASS file.
func ParseFile(path string) (*Script, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Parse(f)
}

// Parse reads an ASS/SSA document. Unknown sections and non-dialogue events
// are ignored; malformed style/event rows are reported in Script.Warnings.
func Parse(r io.Reader) (*Script, error) {
	script := &Script{
		Info:   ScriptInfo{PlayResX: 384, PlayResY: 288},
		Styles: make(map[string]Style),
	}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)

	section := ""
	styleFormat := append([]string(nil), defaultStyleFormat...)
	eventFormat := append([]string(nil), defaultEventFormat...)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if lineNo == 1 {
			line = strings.TrimPrefix(line, "\ufeff")
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, ";") {
			continue
		}
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			section = strings.ToLower(strings.TrimSpace(trimmed[1 : len(trimmed)-1]))
			continue
		}
		key, value, ok := splitKeyValue(line)
		if !ok {
			continue
		}
		switch section {
		case "script info":
			parseScriptInfo(&script.Info, key, value)
		case "v4+ styles", "v4 styles":
			switch key {
			case "format":
				styleFormat = parseFormat(value)
			case "style":
				style, err := parseStyle(value, styleFormat, lineNo)
				if err != nil {
					script.Warnings = append(script.Warnings, fmt.Sprintf("line %d: %v", lineNo, err))
					continue
				}
				script.StyleList = append(script.StyleList, style)
				script.Styles[strings.ToLower(style.Name)] = style
			}
		case "events":
			switch key {
			case "format":
				eventFormat = parseFormat(value)
			case "dialogue":
				event, err := parseEvent(value, eventFormat, lineNo)
				if err != nil {
					script.Warnings = append(script.Warnings, fmt.Sprintf("line %d: %v", lineNo, err))
					continue
				}
				script.Events = append(script.Events, event)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	switch {
	case !script.Info.PlayResXSet && !script.Info.PlayResYSet:
		script.Warnings = append(script.Warnings,
			"PlayResX/PlayResY missing; using ASS defaults 384x288")
	case !script.Info.PlayResXSet:
		script.Warnings = append(script.Warnings,
			"PlayResX missing; using ASS default 384")
	case !script.Info.PlayResYSet:
		script.Warnings = append(script.Warnings,
			"PlayResY missing; using ASS default 288")
	}
	return script, nil
}

func splitKeyValue(line string) (string, string, bool) {
	i := strings.IndexByte(line, ':')
	if i < 0 {
		return "", "", false
	}
	return strings.ToLower(strings.TrimSpace(line[:i])), strings.TrimSpace(line[i+1:]), true
}

func parseScriptInfo(info *ScriptInfo, key, value string) {
	switch key {
	case "playresx":
		if n, ok := parseFloat(value); ok {
			info.PlayResX, info.PlayResXSet = n, true
		}
	case "playresy":
		if n, ok := parseFloat(value); ok {
			info.PlayResY, info.PlayResYSet = n, true
		}
	case "scaledborderandshadow":
		info.ScaledBorderAndShadow = strings.EqualFold(value, "yes") || value == "1"
	case "wrapstyle":
		if n, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
			info.WrapStyle = n
		}
	}
}

func parseFormat(value string) []string {
	parts := strings.Split(value, ",")
	for i := range parts {
		parts[i] = strings.ToLower(strings.TrimSpace(parts[i]))
	}
	return parts
}

func parseStyle(value string, format []string, line int) (Style, error) {
	fields := splitFields(value, len(format))
	get := fieldGetter(format, fields)
	style := Style{
		ScaleX:    100,
		ScaleY:    100,
		Alignment: 2,
		Line:      line,
	}
	style.Name = get("name")
	if style.Name == "" {
		return style, fmt.Errorf("style has no name")
	}
	style.FontName = get("fontname")
	style.FontSize = getFloat(get("fontsize"), 0)
	style.ScaleX = getFloat(get("scalex"), 100)
	style.ScaleY = getFloat(get("scaley"), 100)
	style.Spacing = getFloat(get("spacing"), 0)
	style.Outline = getFloat(get("outline"), 0)
	style.Shadow = getFloat(get("shadow"), 0)
	style.Alignment = int(getFloat(get("alignment"), 2))
	style.MarginL = getFloat(get("marginl"), 0)
	style.MarginR = getFloat(get("marginr"), 0)
	style.MarginV = getFloat(get("marginv"), 0)
	return style, nil
}

func parseEvent(value string, format []string, line int) (Event, error) {
	fields := splitFields(value, len(format))
	get := fieldGetter(format, fields)
	start, err := ParseTime(get("start"))
	if err != nil {
		return Event{}, fmt.Errorf("bad start time: %w", err)
	}
	end, err := ParseTime(get("end"))
	if err != nil {
		return Event{}, fmt.Errorf("bad end time: %w", err)
	}
	return Event{
		Layer:   int(getFloat(get("layer"), 0)),
		Start:   start,
		End:     end,
		Style:   get("style"),
		Name:    get("name"),
		MarginL: getFloat(get("marginl"), 0),
		MarginR: getFloat(get("marginr"), 0),
		MarginV: getFloat(get("marginv"), 0),
		Effect:  get("effect"),
		Text:    get("text"),
		Line:    line,
	}, nil
}

func splitFields(value string, n int) []string {
	if n <= 0 {
		n = 1
	}
	parts := strings.SplitN(value, ",", n)
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func fieldGetter(format, values []string) func(string) string {
	index := make(map[string]int, len(format))
	for i, name := range format {
		if _, exists := index[name]; !exists {
			index[name] = i
		}
	}
	return func(name string) string {
		i, ok := index[name]
		if !ok || i >= len(values) {
			return ""
		}
		return values[i]
	}
}

func getFloat(value string, fallback float64) float64 {
	n, ok := parseFloat(value)
	if !ok {
		return fallback
	}
	return n
}

func parseFloat(value string) (float64, bool) {
	n, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	return n, err == nil
}

// ParseTime parses ASS timestamps such as 0:01:02.34.
func ParseTime(value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	parts := strings.Split(value, ":")
	if len(parts) != 3 {
		return 0, fmt.Errorf("expected H:MM:SS.cc, got %q", value)
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, err
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, err
	}
	sec, err := strconv.ParseFloat(parts[2], 64)
	if err != nil {
		return 0, err
	}
	return time.Duration(h)*time.Hour +
		time.Duration(m)*time.Minute +
		time.Duration(sec*float64(time.Second)), nil
}
