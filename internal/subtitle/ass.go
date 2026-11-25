package subtitle

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

type assStyle struct {
	name string
	font string
}

var (
	assOverride = regexp.MustCompile(`\{[^}]*\}`)
	assFn       = regexp.MustCompile(`(?i)\\fn\s*([^\\}]+)`)
)

// parseASS parses Advanced SubStation Alpha and the older SSA dialect.
func parseASS(text string) (*File, error) {
	f := &File{Format: FormatASS}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")

	styles := map[string]assStyle{}
	var styleOrder []string
	var eventCols []string
	section := ""

	for _, ln := range lines {
		trimmed := strings.TrimSpace(ln)
		if trimmed == "" || strings.HasPrefix(trimmed, ";") {
			continue
		}
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			section = strings.ToLower(strings.TrimSpace(trimmed[1 : len(trimmed)-1]))
			continue
		}
		switch {
		case section == "v4+ styles" || section == "v4 styles":
			if strings.HasPrefix(strings.ToLower(trimmed), "format:") {
				styleOrder = splitASSFields(strings.TrimPrefix(trimmed[len("format:"):], " "))
				continue
			}
			if strings.HasPrefix(strings.ToLower(trimmed), "style:") {
				fields := splitASSFields(trimmed[len("style:"):])
				st := mapASSFields(styleOrder, fields)
				name := st["name"]
				font := st["fontname"]
				if name == "" {
					name = "Style" + strconv.Itoa(len(styles)+1)
				}
				styles[name] = assStyle{name: name, font: font}
				if font != "" {
					f.Fonts = appendUniqueFont(f.Fonts, FontRef{
						Name:   font,
						Source: "style:" + name,
					})
				}
			}
		case section == "events":
			if strings.HasPrefix(strings.ToLower(trimmed), "format:") {
				eventCols = splitASSFields(strings.TrimPrefix(trimmed[len("format:"):], " "))
				continue
			}
			if strings.HasPrefix(strings.ToLower(trimmed), "dialogue:") {
				cue, ok := parseASSDialogue(trimmed[len("dialogue:"):], eventCols, styles)
				if ok {
					cue.Index = len(f.Cues) + 1
					f.Cues = append(f.Cues, cue)
				}
			}
		}
	}
	// Surface per-cue inline \fn overrides at file level so the audit can
	// report every font the script asks the renderer for.
	for _, cue := range f.Cues {
		for _, ref := range cue.Fonts {
			if ref.Source == "inline:fn" {
				f.Fonts = appendUniqueFont(f.Fonts, ref)
			}
		}
	}
	return f, nil
}

func appendUniqueFont(existing []FontRef, ref FontRef) []FontRef {
	for _, e := range existing {
		if e.Name == ref.Name && e.Source == ref.Source {
			return existing
		}
	}
	return append(existing, ref)
}

func splitASSFields(s string) []string {
	parts := strings.Split(s, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func mapASSFields(cols, vals []string) map[string]string {
	out := map[string]string{}
	for i, c := range cols {
		key := strings.ToLower(strings.TrimSpace(c))
		if i < len(vals) {
			out[key] = vals[i]
		}
	}
	return out
}

func parseASSDialogue(body string, cols []string, styles map[string]assStyle) (Cue, bool) {
	if len(cols) == 0 {
		cols = []string{"layer", "start", "end", "style", "name",
			"marginl", "marginr", "marginv", "effect", "text"}
	}
	// Text is the last field and may itself contain commas, so split only up
	// to the column count.
	maxSplit := len(cols) - 1
	fields := strings.SplitN(body, ",", maxSplit+1)
	row := mapASSFields(cols, fields)

	start, err1 := parseASSTime(row["start"])
	end, err2 := parseASSTime(row["end"])
	if err1 != nil || err2 != nil {
		return Cue{}, false
	}

	rawText := row["text"]
	if len(fields) > maxSplit {
		rawText = strings.TrimSpace(fields[maxSplit])
	}

	cue := Cue{
		Index:   0,
		Start:   start,
		End:     end,
		Style:   row["style"],
		RawText: rawText,
		Visible: stripASS(rawText),
	}

	if st, ok := styles[row["style"]]; ok && st.font != "" {
		cue.Fonts = append(cue.Fonts, FontRef{
			Name:   st.font,
			Source: "style:" + st.name,
		})
	}
	for _, m := range assFn.FindAllStringSubmatch(rawText, -1) {
		cue.Fonts = append(cue.Fonts, FontRef{
			Name:   strings.TrimSpace(m[1]),
			Source: "inline:fn",
		})
	}
	return cue, true
}

func parseASSTime(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
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

// stripASS converts override blocks: \N/\n become newlines, \h becomes a
// non-breaking space, drawing tags are deleted but \fn text is not a glyph.
func stripASS(s string) string {
	// Hard/soft line breaks and hard spaces live outside override braces in
	// ASS; convert them before stripping the {...} drawing tags.
	s = strings.ReplaceAll(s, "\\N", "\n")
	s = strings.ReplaceAll(s, "\\n", "\n")
	s = strings.ReplaceAll(s, "\\h", "\u00A0")
	s = assOverride.ReplaceAllStringFunc(s, func(block string) string {
		var b strings.Builder
		return b.String()
	})
	return strings.TrimSpace(s)
}
