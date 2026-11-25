package ass

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// ParseOverride extracts layout tags, clips, and visible text from an ASS
// dialogue body. The returned warnings are non-fatal and are attached to the
// cue report.
func ParseOverride(text string) (Override, []string) {
	var out Override
	var warnings []string
	var visible strings.Builder

	for i := 0; i < len(text); {
		if text[i] != '{' {
			visible.WriteByte(text[i])
			i++
			continue
		}
		end := strings.IndexByte(text[i+1:], '}')
		if end < 0 {
			warnings = append(warnings, "unclosed override block; treating rest as text")
			visible.WriteString(text[i:])
			break
		}
		block := text[i+1 : i+1+end]
		warnings = append(warnings, parseOverrideBlock(block, &out)...)
		i += end + 2
	}
	out.VisibleText = normalizeVisibleText(visible.String())
	return out, warnings
}

func normalizeVisibleText(s string) string {
	s = strings.ReplaceAll(s, `\N`, "\n")
	s = strings.ReplaceAll(s, `\n`, "\n")
	s = strings.ReplaceAll(s, `\h`, " ")
	return s
}

func parseOverrideBlock(block string, out *Override) []string {
	var warnings []string
	for i := 0; i < len(block); {
		if block[i] != '\\' {
			i++
			continue
		}
		start := i + 1
		depth := 0
		j := start
		for ; j < len(block); j++ {
			switch block[j] {
			case '(':
				depth++
			case ')':
				if depth > 0 {
					depth--
				}
			case '\\':
				if depth == 0 {
					goto tokenReady
				}
			}
		}
	tokenReady:
		token := strings.TrimSpace(block[start:j])
		if token != "" {
			if err := applyTag(token, out); err != nil {
				warnings = append(warnings, err.Error())
			}
		}
		i = j
	}
	return warnings
}

func applyTag(token string, out *Override) error {
	name, value := splitTag(token)
	switch name {
	case "clip", "iclip":
		clip, err := parseClip(value)
		if err != nil {
			return fmt.Errorf("\\%s: %w", name, err)
		}
		if name == "iclip" {
			clip.Mode = ClipInverse
		}
		clip.Source = `\` + token
		out.Clips = append(out.Clips, clip)
	case "an":
		n, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || n < 1 || n > 9 {
			return fmt.Errorf("\\an: invalid alignment %q", value)
		}
		out.HasAlignment, out.Alignment = true, n
	case "fs":
		if n, ok := parseTagFloat(value); ok {
			out.FontSize = &n
		}
	case "fscx":
		if n, ok := parseTagFloat(value); ok {
			out.ScaleX = &n
		}
	case "fscy":
		if n, ok := parseTagFloat(value); ok {
			out.ScaleY = &n
		}
	case "bord":
		if n, ok := parseTagFloat(value); ok {
			out.Outline = &n
		}
	case "shad":
		if n, ok := parseTagFloat(value); ok {
			out.Shadow = &n
		}
	case "pos":
		nums := parseTagNumbers(value)
		if len(nums) >= 2 {
			out.Position = &Point{X: nums[0], Y: nums[1]}
		}
	case "move":
		nums := parseTagNumbers(value)
		if len(nums) >= 4 {
			out.MoveFrom = &Point{X: nums[0], Y: nums[1]}
			out.MoveTo = &Point{X: nums[2], Y: nums[3]}
		}
	}
	return nil
}

func splitTag(token string) (string, string) {
	if i := strings.IndexByte(token, '('); i >= 0 {
		end := strings.LastIndexByte(token, ')')
		if end < i {
			end = len(token)
		}
		return strings.ToLower(strings.TrimSpace(token[:i])),
			strings.TrimSpace(token[i+1 : end])
	}
	i := 0
	for i < len(token) {
		if !unicode.IsLetter(rune(token[i])) {
			break
		}
		i++
	}
	return strings.ToLower(strings.TrimSpace(token[:i])),
		strings.TrimSpace(token[i:])
}

func parseTagFloat(value string) (float64, bool) {
	value = strings.TrimSpace(strings.TrimSuffix(value, ")"))
	if value == "" {
		return 0, false
	}
	n, err := strconv.ParseFloat(value, 64)
	return n, err == nil
}

func parseTagNumbers(value string) []float64 {
	value = strings.ReplaceAll(value, "(", "")
	value = strings.ReplaceAll(value, ")", "")
	parts := strings.Split(value, ",")
	nums := make([]float64, 0, len(parts))
	for _, part := range parts {
		n, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil {
			return nil
		}
		nums = append(nums, n)
	}
	return nums
}
