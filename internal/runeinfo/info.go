// Package runeinfo annotates runes with their Unicode block, script and
// safe substitution candidates. Substitutions are suggestions for a human
// reviewer; the audit never rewrites the subtitle on its own.
package runeinfo

import (
	"fmt"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Info describes one missing rune in report-friendly terms.
type Info struct {
	Rune       rune   `json:"rune"`
	Codepoint  string `json:"codepoint"`
	Display    string `json:"display"`
	Block      string `json:"block"`
	Script     string `json:"script"`
	Category   string `json:"category"`
	PunctAlias string `json:"punctAlias,omitempty"`
}

type blockRange struct {
	start, end rune
	name       string
	script     string
}

// Ranges limited to the blocks that show up in subtitle work; anything
// outside the table falls back to a generic plane label.
var blocks = []blockRange{
	{0x0020, 0x007F, "Basic Latin", "Latin"},
	{0x00A0, 0x00FF, "Latin-1 Supplement", "Latin"},
	{0x0100, 0x017F, "Latin Extended-A", "Latin"},
	{0x0180, 0x024F, "Latin Extended-B", "Latin"},
	{0x0370, 0x03FF, "Greek and Coptic", "Greek"},
	{0x1F00, 0x1FFF, "Greek Extended", "Greek"},
	{0x0400, 0x04FF, "Cyrillic", "Cyrillic"},
	{0x2000, 0x206F, "General Punctuation", "Common"},
	{0x2070, 0x209F, "Superscripts and Subscripts", "Common"},
	{0x20A0, 0x20CF, "Currency Symbols", "Common"},
	{0x2100, 0x214F, "Letterlike Symbols", "Common"},
	{0x2150, 0x218F, "Number Forms", "Common"},
	{0x2190, 0x21FF, "Arrows", "Common"},
	{0x2200, 0x22FF, "Mathematical Operators", "Common"},
	{0x2460, 0x24FF, "Enclosed Alphanumerics", "Common"},
	{0x25A0, 0x25FF, "Geometric Shapes", "Common"},
	{0x2600, 0x26FF, "Miscellaneous Symbols", "Common"},
	{0x3000, 0x303F, "CJK Symbols and Punctuation", "Han"},
	{0x3040, 0x309F, "Hiragana", "Hiragana"},
	{0x30A0, 0x30FF, "Katakana", "Katakana"},
	{0x3400, 0x4DBF, "CJK Unified Ideographs Extension A", "Han"},
	{0x4E00, 0x9FFF, "CJK Unified Ideographs", "Han"},
	{0xF900, 0xFAFF, "CJK Compatibility Ideographs", "Han"},
	{0xFE30, 0xFE4F, "CJK Compatibility Forms", "Han"},
	{0xFF00, 0xFFEF, "Halfwidth and Fullwidth Forms", "Common"},
}

// Lookup annotates a rune.
func Lookup(r rune) Info {
	info := Info{
		Rune:      r,
		Codepoint: fmt.Sprintf("U+%04X", r),
		Display:   displayRune(r),
		Block:     genericBlock(r),
		Script:    "Unknown",
		Category:  CategoryName(r),
	}
	for _, b := range blocks {
		if r >= b.start && r <= b.end {
			info.Block = b.name
			info.Script = b.script
			break
		}
	}
	info.PunctAlias = punctAlias(r)
	return info
}

func genericBlock(r rune) string {
	switch {
	case r <= 0xFFFF:
		return "BMP (unlisted block)"
	case r <= 0x1FFFF:
		return "Supplementary Multilingual Plane"
	default:
		return "Supplementary Plane"
	}
}

func displayRune(r rune) string {
	if unicode.IsPrint(r) {
		return string(r)
	}
	return "<non-printable>"
}

// CategoryName returns the Unicode general category using the stdlib range
// tables, avoiding a full Unicode name database dependency.
func CategoryName(r rune) string {
	switch {
	case unicode.Is(unicode.Lu, r):
		return "Lu (uppercase letter)"
	case unicode.Is(unicode.Ll, r):
		return "Ll (lowercase letter)"
	case unicode.Is(unicode.Lt, r):
		return "Lt (titlecase letter)"
	case unicode.Is(unicode.Lm, r):
		return "Lm (modifier letter)"
	case unicode.Is(unicode.Lo, r):
		return "Lo (other letter)"
	case unicode.Is(unicode.Mn, r):
		return "Mn (nonspacing mark)"
	case unicode.Is(unicode.Mc, r):
		return "Mc (spacing mark)"
	case unicode.Is(unicode.Nd, r):
		return "Nd (decimal number)"
	case unicode.Is(unicode.Pd, r):
		return "Pd (dash punctuation)"
	case unicode.Is(unicode.Ps, r):
		return "Ps (open punctuation)"
	case unicode.Is(unicode.Pe, r):
		return "Pe (close punctuation)"
	case unicode.Is(unicode.Pi, r):
		return "Pi (initial quote)"
	case unicode.Is(unicode.Pf, r):
		return "Pf (final quote)"
	case unicode.Is(unicode.Pc, r):
		return "Pc (connector punctuation)"
	case unicode.Is(unicode.Po, r):
		return "Po (other punctuation)"
	case unicode.Is(unicode.Sm, r):
		return "Sm (math symbol)"
	case unicode.Is(unicode.Sc, r):
		return "Sc (currency symbol)"
	case unicode.Is(unicode.So, r):
		return "So (other symbol)"
	case unicode.Is(unicode.Cs, r):
		return "Cs (surrogate)"
	case unicode.IsControl(r):
		return "Cc (control)"
	case unicode.IsSpace(r):
		return "Zs (space separator)"
	case unicode.Is(unicode.So, r):
		return "So (other symbol)"
	case unicode.Is(unicode.No, r):
		return "No (other number)"
	case unicode.Is(unicode.Nl, r):
		return "Nl (letter number)"
	case unicode.Is(unicode.Me, r):
		return "Me (enclosing mark)"
	case unicode.Is(unicode.Sk, r):
		return "Sk (modifier symbol)"
	case unicode.Is(unicode.Co, r):
		return "Co (private use)"
	default:
		return "Cn/other (unassigned or private use)"
	}
}

// punctAliasMap maps typographic punctuation that burn-in fonts commonly
// lack onto an ASCII/CJK-basic equivalent the target font is likely to have.
var punctAliasMap = map[rune]string{
	0x2018: "'", 0x2019: "'", 0x201A: "'", 0x201B: "'",
	0x201C: "\"", 0x201D: "\"", 0x201E: "\"", 0x201F: "\"",
	0x2013: "-", 0x2014: "--", 0x2015: "--",
	0x2026: "...",
	0x00A0: " ",
	0x2009: " ", 0x200A: " ", 0x202F: " ",
	0x2002: " ", 0x2003: " ",
	0x2022: "*", 0x2027: "*",
	0x00D7: "x", 0x00F7: "/",
	0x2212: "-",
	0x3000: " ",
	0xFF01: "!", 0xFF1F: "?", 0xFF0C: ",", 0xFF1A: ":",
	0xFF1B: ";", 0xFF08: "(", 0xFF09: ")", 0xFF05: "%",
	0xFF0E: ".",
}

func punctAlias(r rune) string {
	if a, ok := punctAliasMap[r]; ok {
		return a
	}
	return ""
}

// NFKCFold returns the NFKC compatibility decomposition when it differs from
// the input. Fullwidth/halfwidth forms, ligatures and enclosed numerals all
// fold onto base characters that older fonts usually cover.
func NFKCFold(r rune) (folded string, ok bool) {
	s := string(r)
	f := norm.NFKC.String(s)
	if f == s || f == "" {
		return "", false
	}
	return f, true
}
