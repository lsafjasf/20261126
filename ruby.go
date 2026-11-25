package cuemap

import (
	"html"
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	htmlRubyRE = regexp.MustCompile(`(?is)<ruby\b[^>]*>(.*?)<rt\b[^>]*>(.*?)</rt>(?:\s*<rp\b[^>]*>.*?</rp>)?\s*</ruby>`)
	rpRE       = regexp.MustCompile(`(?is)<rp\b[^>]*>.*?</rp>`)
	tagRE      = regexp.MustCompile(`(?s)<[^>]+>`)
	brRE       = regexp.MustCompile(`(?i)<br\s*/?>`)
)

// ParseCue recognizes the small ruby markup dialects used by subtitle fixtures.
// It preserves raw in Cue.Raw, while Text contains only logical base text.
func ParseCue(id, raw string) Cue {
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, `\N`, "\n")

	if htmlRubyRE.MatchString(normalized) {
		text, spans := parseHTMLRuby(normalized)
		return Cue{ID: id, Text: text, Raw: raw, Ruby: spans}
	}
	if strings.Contains(normalized, "《") && (strings.Contains(normalized, "｜") || strings.Contains(normalized, "|")) {
		text, spans := ParseAozoraRuby(normalized)
		return Cue{ID: id, Text: text, Raw: raw, Ruby: spans}
	}
	return Cue{ID: id, Text: plainSubtitleText(normalized), Raw: raw}
}

func parseHTMLRuby(raw string) (string, []RubySpan) {
	var text strings.Builder
	spans := make([]RubySpan, 0)
	last := 0

	for _, match := range htmlRubyRE.FindAllStringSubmatchIndex(raw, -1) {
		text.WriteString(plainSubtitleText(raw[last:match[0]]))

		baseHTML := raw[match[2]:match[3]]
		rubyHTML := raw[match[4]:match[5]]
		base := plainInlineText(baseHTML)
		annotation := plainInlineText(rubyHTML)

		start := utf8.RuneCountInString(text.String())
		text.WriteString(base)
		end := utf8.RuneCountInString(text.String())
		if annotation != "" && end > start {
			spans = append(spans, RubySpan{
				StartRune: start,
				EndRune:   end,
				Base:      base,
				Text:      annotation,
			})
		}
		last = match[1]
	}
	text.WriteString(plainSubtitleText(raw[last:]))
	return text.String(), spans
}

// ParseAozoraRuby parses ｜Base《annotation》 or |Base《annotation》.
// The full base interval is the text between the marker and 《.
func ParseAozoraRuby(input string) (string, []RubySpan) {
	var text strings.Builder
	spans := make([]RubySpan, 0)
	runes := []rune(input)

	for i := 0; i < len(runes); {
		if (runes[i] == '｜' || runes[i] == '|') && i+1 < len(runes) {
			startMark := i
			book := indexRune(runes, '《', i+1)
			if book > 0 {
				closeBook := indexRune(runes, '》', book+1)
				if closeBook > book {
					base := string(runes[i+1 : book])
					annotation := string(runes[book+1 : closeBook])
					text.WriteString(string(runes[:startMark]))
					_ = startMark
					offset := utf8.RuneCountInString(text.String())
					text.WriteString(base)
					if annotation != "" && base != "" {
						spans = append(spans, RubySpan{
							StartRune: offset,
							EndRune:   offset + utf8.RuneCountInString(base),
							Base:      base,
							Text:      annotation,
						})
					}
					runes = append(append([]rune{}, runes[:0]...), runes[closeBook+1:]...)
					i = 0
					continue
				}
			}
		}
		i++
	}
	text.WriteString(string(runes))
	return text.String(), spans
}

func indexRune(runes []rune, target rune, start int) int {
	for i := start; i < len(runes); i++ {
		if runes[i] == target {
			return i
		}
	}
	return -1
}

func plainSubtitleText(s string) string {
	s = brRE.ReplaceAllString(s, "\n")
	return plainInlineText(s)
}

func plainInlineText(s string) string {
	s = rpRE.ReplaceAllString(s, "")
	s = tagRE.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	return s
}

// ValidateRuby checks rune bounds and rejects overlapping attachments.
func ValidateRuby(cue Cue) error {
	total := utf8.RuneCountInString(cue.Text)
	type interval struct{ start, end int }
	seen := make([]interval, 0, len(cue.Ruby))
	for i, span := range cue.Ruby {
		if span.Text == "" {
			return &LayoutError{Code: FallbackEmptyRuby, CueID: cue.ID, Message: "ruby annotation is empty"}
		}
		if span.StartRune < 0 || span.EndRune <= span.StartRune || span.EndRune > total {
			return &LayoutError{Code: FallbackInvalidRuby, CueID: cue.ID, Message: "ruby rune interval is outside cue text"}
		}
		for _, other := range seen {
			if span.StartRune < other.end && other.start < span.EndRune {
				return &LayoutError{Code: FallbackOverlapRuby, CueID: cue.ID, Message: "ruby intervals overlap"}
			}
		}
		seen = append(seen, interval{span.StartRune, span.EndRune})
		_ = i
	}
	return nil
}

// LayoutError describes a condition for which the caller should use the
// original cue. It deliberately carries the cue identifier.
type LayoutError struct {
	Code    FallbackReason `json:"code"`
	CueID   string         `json:"cue_id"`
	Message string         `json:"message"`
}

func (e *LayoutError) Error() string {
	if e.CueID == "" {
		return string(e.Code) + ": " + e.Message
	}
	return "cue " + e.CueID + ": " + string(e.Code) + ": " + e.Message
}
