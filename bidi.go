package cuemap

import (
	"unicode"
	"unicode/utf8"
)

// Direction selects the paragraph base direction.
type Direction int

const (
	// DirectionAuto uses the first strong LTR/RTL character.
	DirectionAuto Direction = iota
	DirectionLTR
	DirectionRTL
)

// BidiClass is the small subset of Unicode bidi character classes used by the
// subtitle-oriented resolver.
type BidiClass uint8

const (
	BidiUnknown BidiClass = iota
	BidiL
	BidiR
	BidiAL
	BidiEN
	BidiAN
	BidiES
	BidiCS
	BidiNSM
	BidiWS
	BidiON
)

func (c BidiClass) String() string {
	switch c {
	case BidiL:
		return "L"
	case BidiR:
		return "R"
	case BidiAL:
		return "AL"
	case BidiEN:
		return "EN"
	case BidiAN:
		return "AN"
	case BidiES:
		return "ES"
	case BidiCS:
		return "CS"
	case BidiNSM:
		return "NSM"
	case BidiWS:
		return "WS"
	default:
		return "ON"
	}
}

// BidiClassOf classifies a rune for the built-in bidi resolver.
func BidiClassOf(r rune) BidiClass {
	switch {
	case r == 0x200E: // LRM
		return BidiL
	case r == 0x200F: // RLM
		return BidiR
	case r == 0x061C: // ALM
		return BidiAL
	case r == 0x202C || r == 0x2069: // PDF/PDI
		return BidiON
	case r >= 0x202A && r <= 0x202E:
		if r == 0x202A || r == 0x202D {
			return BidiL
		}
		return BidiR
	case r == 0x2066: // LRI
		return BidiL
	case r == 0x2067: // RLI
		return BidiR
	case r >= '0' && r <= '9':
		return BidiEN
	case r >= 0x0660 && r <= 0x0669:
		return BidiAN
	case r >= 0x06F0 && r <= 0x06F9:
		return BidiEN
	case IsCombiningRune(r):
		return BidiNSM
	case r == '+' || r == '-':
		return BidiES
	case r == ',' || r == ':' || r == ';' || r == 0x060C || r == 0x061B:
		return BidiCS
	case unicode.IsSpace(r):
		return BidiWS
	case isArabicLetter(r):
		return BidiAL
	case isHebrewLetter(r):
		return BidiR
	case unicode.IsLetter(r):
		return BidiL
	case unicode.IsDigit(r):
		return BidiEN
	default:
		return BidiON
	}
}

func isArabicLetter(r rune) bool {
	return unicode.In(r, unicode.Arabic) && unicode.IsLetter(r)
}

func isHebrewLetter(r rune) bool {
	return unicode.In(r, unicode.Hebrew) && unicode.IsLetter(r)
}

type strongSide int

const (
	sideNeutral strongSide = iota
	sideLTR
	sideRTL
)

// ResolveBaseDirection reports the selected base direction and level.
func ResolveBaseDirection(line string, forced Direction) (Direction, int) {
	if forced == DirectionLTR {
		return DirectionLTR, 0
	}
	if forced == DirectionRTL {
		return DirectionRTL, 1
	}
	for _, r := range line {
		class := BidiClassOf(r)
		if class == BidiR || class == BidiAL {
			return DirectionRTL, 1
		}
		if class == BidiL {
			return DirectionLTR, 0
		}
	}
	return DirectionLTR, 0
}

// MapLine builds the logical-to-screen mapping for one line.
func MapLine(line string, forced Direction) Mapping {
	clusters, runes := ClusterLine(line)
	baseDirection, baseLevel := ResolveBaseDirection(line, forced)
	levels := resolveLevels(clusters, baseLevel)
	screenOrder := reorderLevels(len(clusters), levels)

	logicalToScreen := make([]int, len(clusters))
	for screen, logical := range screenOrder {
		logicalToScreen[logical] = screen
	}

	runeToScreen := make([]int, len(runes))
	screenToRunes := make([][]int, len(screenOrder))
	for runeIndex := range runes {
		clusterIndex := runes[runeIndex].Cluster
		screen := logicalToScreen[clusterIndex]
		runeToScreen[runeIndex] = screen
		screenToRunes[screen] = append(screenToRunes[screen], runeIndex)
	}

	visual := make([]byte, 0, len(line))
	for _, logical := range screenOrder {
		visual = append(visual, clusters[logical].Text...)
	}

	return Mapping{
		Logical:         line,
		Visual:          string(visual),
		BaseDirection:   baseDirection,
		BaseLevel:       baseLevel,
		Levels:          levels,
		Clusters:        clusters,
		Runes:           runes,
		ScreenOrder:     screenOrder,
		LogicalToScreen: logicalToScreen,
		RuneToScreen:    runeToScreen,
		ScreenToRunes:   screenToRunes,
	}
}

// MapText maps every newline-separated cue line independently.
func MapText(text string, forced Direction) []Mapping {
	lines := splitCueLines(text)
	out := make([]Mapping, 0, len(lines))
	for _, line := range lines {
		out = append(out, MapLine(line, forced))
	}
	return out
}

func splitCueLines(text string) []string {
	if text == "" {
		return []string{""}
	}
	out := make([]string, 0)
	start := 0
	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		if r == '\n' {
			lineEnd := i
			if lineEnd > start && text[lineEnd-1] == '\r' {
				lineEnd--
			}
			out = append(out, text[start:lineEnd])
			start = i + size
		}
		i += size
	}
	lineEnd := len(text)
	if lineEnd > start && text[lineEnd-1] == '\r' {
		lineEnd--
	}
	return append(out, text[start:lineEnd])
}

func resolveLevels(clusters []Cluster, baseLevel int) []int {
	classes := make([]BidiClass, len(clusters))
	for i, cluster := range clusters {
		classes[i] = cluster.BidiClass
		if classes[i] == BidiNSM && i > 0 {
			classes[i] = classes[i-1]
		}
	}

	resolveNumericSeparators(clusters, classes)

	strong := make([]strongSide, len(clusters))
	for i, class := range classes {
		switch class {
		case BidiL:
			strong[i] = sideLTR
		case BidiR, BidiAL:
			strong[i] = sideRTL
		default:
			strong[i] = sideNeutral
		}
	}

	for start := 0; start < len(clusters); {
		if strong[start] != sideNeutral {
			start++
			continue
		}
		end := start
		for end < len(clusters) && strong[end] == sideNeutral {
			end++
		}
		left := nearestStrong(strong, start-1, -1)
		right := nearestStrong(strong, end, 1)
		resolved := strongSide(baseLevel + 1) // 0 -> LTR, 1 -> RTL
		if left != sideNeutral && left == right {
			resolved = left
		} else if left != sideNeutral && right == sideNeutral {
			resolved = left
		} else if right != sideNeutral && left == sideNeutral {
			resolved = right
		}
		for i := start; i < end; i++ {
			strong[i] = resolved
		}
		start = end
	}

	levels := make([]int, len(clusters))
	for i, class := range classes {
		switch class {
		case BidiEN, BidiAN:
			levels[i] = 2
		case BidiL:
			if baseLevel == 1 {
				levels[i] = 2
			} else {
				levels[i] = baseLevel
			}
		case BidiR, BidiAL:
			if baseLevel == 0 {
				levels[i] = 1
			} else {
				levels[i] = baseLevel
			}
		default:
			if strong[i] == sideRTL {
				if baseLevel == 0 {
					levels[i] = 1
				} else {
					levels[i] = baseLevel
				}
			} else {
				if baseLevel == 1 {
					levels[i] = 2
				} else {
					levels[i] = baseLevel
				}
			}
		}
	}
	return levels
}

func resolveNumericSeparators(clusters []Cluster, classes []BidiClass) {
	for i := 1; i+1 < len(classes); i++ {
		if classes[i] != BidiES && classes[i] != BidiCS {
			continue
		}
		if classes[i-1] == BidiEN && classes[i+1] == BidiEN {
			classes[i] = BidiEN
			continue
		}
		if classes[i] == BidiCS && (classes[i-1] == BidiAN || classes[i+1] == BidiAN) {
			classes[i] = BidiAN
		}
	}
}

func nearestStrong(strong []strongSide, start int, step int) strongSide {
	for i := start; i >= 0 && i < len(strong); i += step {
		if strong[i] != sideNeutral {
			return strong[i]
		}
	}
	return sideNeutral
}

func reorderLevels(n int, levels []int) []int {
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	if n == 0 {
		return order
	}

	maxLevel := 0
	for _, level := range levels {
		if level > maxLevel {
			maxLevel = level
		}
	}

	for level := maxLevel; level >= 1; level-- {
		for start := 0; start < n; {
			if levels[order[start]] < level {
				start++
				continue
			}
			end := start
			for end < n && levels[order[end]] >= level {
				end++
			}
			reverseInts(order[start:end])
			start = end
		}
	}
	return order
}

func reverseInts(values []int) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}
