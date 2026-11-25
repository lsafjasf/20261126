package medref

import (
	"strings"
	"unicode"
)

// CaseInsensitive reports whether the volume holding p is case-insensitive
// on the given platform. Drive letters and UNC shares are Windows volumes.
func (p Path) CaseInsensitive(plat Platform) bool {
	switch p.Kind {
	case WindowsDrive, UNC:
		return true
	case Relative:
		return plat != Linux
	case UnixRooted:
		return plat == Darwin
	default:
		return false
	}
}

// LexicallyEqual reports whether two canonical keys describe the same
// reference according to the platform's case policy. It never folds
// non-ASCII characters, so distinct Unicode spellings stay distinct.
func LexicallyEqual(a, b Path, plat Platform) bool {
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case WindowsDrive:
		return a.Drive == b.Drive && foldEqual(a.Clean, b.Clean, true)
	case UNC:
		return strings.EqualFold(a.Server, b.Server) &&
			strings.EqualFold(a.Share, b.Share) &&
			foldEqual(a.Clean, b.Clean, true)
	case UnixRooted:
		return foldEqual(a.Clean, b.Clean, plat == Darwin)
	case Relative:
		return foldEqual(a.Clean, b.Clean, plat != Linux)
	default:
		return a.Canonical == b.Canonical
	}
}

func foldEqual(a, b string, fold bool) bool {
	if fold {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// LooseEqual is a filesystem-probing helper used only when a referenced
// file cannot be found at its literal spelling. It treats two segments as
// equal when ASCII case differs, or when they differ only by Unicode
// normalization form (combining marks and Hangul Jamo composition).
func LooseEqual(a, b string) bool {
	if a == b {
		return true
	}
	if strings.EqualFold(a, b) {
		return true
	}
	return looseUnicode(a) == looseUnicode(b)
}

// looseUnicode canonicalizes a segment for filesystem probing: combining
// mark sequences are composed back into precomposed letters (NFC-style),
// and precomposed Hangul syllables are expanded into Jamo. Marks without
// a known composition are kept, so e-acute and e-grave never collapse.
func looseUnicode(s string) string {
	runes := []rune(s)
	var b strings.Builder
	b.Grow(len(s) + 8)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r >= hangulSyllableBase && r <= hangulSyllableBase+hangulSyllableCount-1 {
			writeJamo(&b, r)
			continue
		}
		if i+1 < len(runes) && unicode.In(runes[i+1], unicode.Mn, unicode.Mc, unicode.Me) {
			if composed, ok := composeMark(r, runes[i+1]); ok {
				b.WriteRune(composed)
				i++
				continue
			}
		}
		b.WriteRune(r)
	}
	return b.String()
}

// composeMark maps a base letter plus one combining mark to the
// precomposed Latin-1 letter. Unknown pairs report false so the raw
// characters survive the comparison instead of being silently merged.
func composeMark(base, mark rune) (rune, bool) {
	r, ok := nfcMap[markPair{base, mark}]
	return r, ok
}

type markPair struct{ base, mark rune }

// nfcMap covers the accented Latin-1 letters commonly seen in file names.
// Combining marks: acute U+0301, grave U+0300, circumflex U+0302,
// tilde U+0303, diaeresis U+0308, ring U+030A, cedilla U+0327.
var nfcMap = buildNFCMap()

func buildNFCMap() map[markPair]rune {
	const (
		acute      = 0x0301
		grave      = 0x0300
		circumflex = 0x0302
		tilde      = 0x0303
		diaeresis  = 0x0308
		ring       = 0x030A
		cedilla    = 0x0327
	)
	type composed struct {
		base, mark, out rune
	}
	rows := []composed{
		{'a', grave, 'à'}, {'a', acute, 'á'}, {'a', circumflex, 'â'},
		{'a', tilde, 'ã'}, {'a', diaeresis, 'ä'}, {'a', ring, 'å'},
		{'c', cedilla, 'ç'},
		{'e', grave, 'è'}, {'e', acute, 'é'}, {'e', circumflex, 'ê'},
		{'e', diaeresis, 'ë'},
		{'i', grave, 'ì'}, {'i', acute, 'í'}, {'i', circumflex, 'î'},
		{'i', diaeresis, 'ï'},
		{'n', tilde, 'ñ'},
		{'o', grave, 'ò'}, {'o', acute, 'ó'}, {'o', circumflex, 'ô'},
		{'o', tilde, 'õ'}, {'o', diaeresis, 'ö'},
		{'u', grave, 'ù'}, {'u', acute, 'ú'}, {'u', circumflex, 'û'},
		{'u', diaeresis, 'ü'},
		{'y', acute, 'ý'}, {'y', diaeresis, 'ÿ'},
	}
	m := make(map[markPair]rune, len(rows)*2)
	for _, row := range rows {
		m[markPair{row.base, row.mark}] = row.out
		m[markPair{toUpper(row.base), row.mark}] = toUpper(row.out)
	}
	return m
}

func toUpper(r rune) rune {
	if r >= 'a' && r <= 'z' {
		return r - ('a' - 'A')
	}
	if r >= 0x00E0 && r <= 0x00FF {
		return r - 32
	}
	return r
}

const (
	hangulSyllableBase  = 0xAC00
	hangulJamoLBase     = 0x1100
	hangulJamoVBase     = 0x1161
	hangulJamoTBase     = 0x11A7
	hangulJamoVCount    = 21
	hangulJamoTCount    = 28
	hangulSyllableCount = 11172
)

func writeJamo(b *strings.Builder, syllable rune) {
	index := int(syllable - hangulSyllableBase)
	l := index / (hangulJamoVCount * hangulJamoTCount)
	v := (index / hangulJamoTCount) % hangulJamoVCount
	t := index % hangulJamoTCount
	b.WriteRune(rune(hangulJamoLBase + l))
	b.WriteRune(rune(hangulJamoVBase + v))
	if t != 0 {
		b.WriteRune(rune(hangulJamoTBase + t))
	}
}
