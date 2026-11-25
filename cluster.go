package cuemap

import (
	"unicode"
	"unicode/utf8"
)

// RuneInfo records one logical Unicode code point in a line.
//
// Index and ByteStart are both zero-based. Cluster identifies the grapheme-like
// unit that owns the rune. A combining mark has the same screen slot as its
// base cluster.
type RuneInfo struct {
	Index     int       `json:"index"`
	ByteStart int       `json:"byte_start"`
	ByteSize  int       `json:"byte_size"`
	Value     rune      `json:"value"`
	Text      string    `json:"text"`
	Cluster   int       `json:"cluster"`
	Combining bool      `json:"combining"`
	BidiClass BidiClass `json:"bidi_class"`
}

// Cluster is a logical character unit that must not be split for display.
// It is a small, deterministic approximation of an extended grapheme cluster:
// a base plus marks/variation selectors, common ZWJ sequences, flag pairs, and
// skin-tone modifiers stay together.
type Cluster struct {
	Index     int       `json:"index"`
	StartRune int       `json:"start_rune"`
	EndRune   int       `json:"end_rune"`
	Runes     []int     `json:"runes"`
	ByteStart int       `json:"byte_start"`
	ByteEnd   int       `json:"byte_end"`
	Text      string    `json:"text"`
	Base      rune      `json:"base"`
	BidiClass BidiClass `json:"bidi_class"`
}

// ClusterLine splits one line (without its newline) into indivisible units.
func ClusterLine(line string) ([]Cluster, []RuneInfo) {
	runes := make([]rune, 0, utf8.RuneCountInString(line))
	starts := make([]int, 0, cap(runes))
	for offset, r := range line {
		starts = append(starts, offset)
		runes = append(runes, r)
	}

	infos := make([]RuneInfo, len(runes))
	for i, r := range runes {
		size := utf8.RuneLen(r)
		if size < 0 {
			size = 1
		}
		infos[i] = RuneInfo{
			Index:     i,
			ByteStart: starts[i],
			ByteSize:  size,
			Value:     r,
			Text:      string(r),
			Combining: IsCombiningRune(r),
			BidiClass: BidiClassOf(r),
		}
	}

	clusters := make([]Cluster, 0)
	for start := 0; start < len(runes); {
		end := start + 1
		if isRegionalIndicator(runes[start]) && end < len(runes) && isRegionalIndicator(runes[end]) {
			end++
		}

		for end < len(runes) {
			r := runes[end]
			switch {
			case IsCombiningRune(r), isVariationSelector(r), isSkinTone(r):
				end++
			case r == 0x200D && end+1 < len(runes):
				end += 2
			default:
				goto clustered
			}
		}
	clustered:
		byteEnd := len(line)
		if end < len(starts) {
			byteEnd = starts[end]
		}
		text := line[starts[start]:byteEnd]
		runeIndexes := append([]int(nil), rangeInterval(start, end)...)
		class := infos[start].BidiClass
		clusterIndex := len(clusters)
		clusters = append(clusters, Cluster{
			Index:     clusterIndex,
			StartRune: start,
			EndRune:   end,
			Runes:     runeIndexes,
			ByteStart: starts[start],
			ByteEnd:   byteEnd,
			Text:      text,
			Base:      runes[start],
			BidiClass: class,
		})
		for runeIndex := start; runeIndex < end; runeIndex++ {
			infos[runeIndex].Cluster = clusterIndex
		}
		start = end
	}

	return clusters, infos
}

// IsCombiningRune reports whether r is a Unicode mark that attaches to a base.
func IsCombiningRune(r rune) bool {
	return unicode.In(r, unicode.Mn, unicode.Me, unicode.Mc)
}

func isVariationSelector(r rune) bool {
	return (r >= 0xFE00 && r <= 0xFE0F) || (r >= 0xE0100 && r <= 0xE01EF)
}

func isSkinTone(r rune) bool {
	return r >= 0x1F3FB && r <= 0x1F3FF
}

func isRegionalIndicator(r rune) bool {
	return r >= 0x1F1E6 && r <= 0x1F1FF
}

func rangeInterval(start, end int) []int {
	out := make([]int, 0, end-start)
	for i := start; i < end; i++ {
		out = append(out, i)
	}
	return out
}
