// Package encodingx decodes subtitle bytes into UTF-8 while preserving
// evidence of how the source was encoded. Rendering tofu is frequently an
// encoding problem (UTF-16 BOM, GB18030 file read as UTF-8, double-encoded
// UTF-8) rather than a missing glyph, so every transformation is reported
// instead of hidden.
package encodingx

import (
	"bytes"
	"encoding/binary"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/unicode"
)

// Kind names the detected source encoding.
type Kind string

const (
	KindUTF8        Kind = "utf-8"
	KindUTF8BOM     Kind = "utf-8-bom"
	KindUTF16LEBOM  Kind = "utf-16le-bom"
	KindUTF16BEBOM  Kind = "utf-16be-bom"
	KindUTF32LEBOM  Kind = "utf-32le-bom"
	KindUTF32BEBOM  Kind = "utf-32be-bom"
	KindUTF16LENOBM Kind = "utf-16le"
	KindUTF16BENOBM Kind = "utf-16be"
	KindGB18030     Kind = "gb18030"
	KindUnknown     Kind = "unknown"
)

// Result carries the decoded text plus every encoding issue found.
type Result struct {
	Encoding         Kind
	Text             string
	NotesList        []string
	HasReplacement   bool
	ReplacementCount int
	NulCount         int
	Mojibake         bool
}

var (
	bomUTF8    = []byte{0xEF, 0xBB, 0xBF}
	bomUTF16LE = []byte{0xFF, 0xFE}
	bomUTF16BE = []byte{0xFE, 0xFF}
	bomUTF32LE = []byte{0xFF, 0xFE, 0x00, 0x00}
	bomUTF32BE = []byte{0x00, 0x00, 0xFE, 0xFF}
)

// Decode detects the encoding and returns UTF-8 text. It never fails on
// invalid bytes: invalid sequences become U+FFFD and are counted so the
// report can show exactly where rendering was corrupted.
func Decode(raw []byte) Result {
	res := Result{}

	switch {
	case bytes.HasPrefix(raw, bomUTF32LE):
		res.Encoding = KindUTF32LEBOM
		res.Text = decodeUTF32(raw[len(bomUTF32LE):], binary.LittleEndian)
	case bytes.HasPrefix(raw, bomUTF32BE):
		res.Encoding = KindUTF32BEBOM
		res.Text = decodeUTF32(raw[len(bomUTF32BE):], binary.BigEndian)
	case bytes.HasPrefix(raw, bomUTF16LE):
		res.Encoding = KindUTF16LEBOM
		res.decodeUTF16(unicode.UTF16(unicode.LittleEndian, unicode.UseBOM), raw)
	case bytes.HasPrefix(raw, bomUTF16BE):
		res.Encoding = KindUTF16BEBOM
		res.decodeUTF16(unicode.UTF16(unicode.BigEndian, unicode.UseBOM), raw)
	case bytes.HasPrefix(raw, bomUTF8):
		res.Encoding = KindUTF8BOM
		res.Text = string(raw[len(bomUTF8):])
	default:
		res.detectNoBOM(raw)
	}

	res.countArtifacts()
	res.detectMojibake()
	return res
}

func (r *Result) decodeUTF16(e encoding.Encoding, raw []byte) {
	out, err := e.NewDecoder().Bytes(raw)
	if err != nil {
		r.NotesList = append(r.NotesList, "decoder error: "+err.Error()+"; invalid bytes replaced with U+FFFD")
	}
	r.Text = string(out)
}

// decodeUTF32 handles UTF-32, which golang.org/x/text does not provide an
// encoding for. Invalid code units become U+FFFD like every other path.
func decodeUTF32(b []byte, order binary.ByteOrder) string {
	var runes []rune
	for i := 0; i+3 < len(b); i += 4 {
		v := order.Uint32(b[i : i+4])
		switch {
		case v == 0:
			runes = append(runes, 0)
		case v > 0x10FFFF || (v >= 0xD800 && v <= 0xDFFF):
			runes = append(runes, utf8.RuneError)
		default:
			runes = append(runes, rune(v))
		}
	}
	return string(runes)
}

func (r *Result) detectNoBOM(raw []byte) {
	// NUL-heavy content with no BOM is almost certainly UTF-16 written by a
	// tool that omitted the BOM. Check this first: ASCII encoded as UTF-16LE
	// is coincidentally valid UTF-8, so an utf8.Valid check would misfire.
	if le, be := nulProfile(raw); le > 0 || be > 0 {
		if le >= be {
			r.Encoding = KindUTF16LENOBM
			r.decodeUTF16(unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM), raw)
		} else {
			r.Encoding = KindUTF16BENOBM
			r.decodeUTF16(unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM), raw)
		}
		r.NotesList = append(r.NotesList, "UTF-16 without BOM detected from NUL byte profile")
		return
	}
	if utf8.Valid(raw) {
		r.Encoding = KindUTF8
		r.Text = string(raw)
		return
	}
	// Chinese Windows tools still emit GB18030/GBK without a BOM.
	if out, err := simplifiedchinese.GB18030.NewDecoder().Bytes(raw); err == nil &&
		utf8.Valid(out) && hanRatio(out) >= 0.1 {
		r.Encoding = KindGB18030
		r.Text = string(out)
		r.NotesList = append(r.NotesList, "source is GB18030/GBK, not UTF-8; a UTF-8-only renderer would show mojibake or tofu")
		return
	}
	r.Encoding = KindUnknown
	r.Text = string(bytes.ToValidUTF8(raw, []byte{0xEF, 0xBF, 0xBD}))
	r.NotesList = append(r.NotesList, "unrecognized encoding; invalid byte sequences replaced with U+FFFD")
}

// nulProfile scores how strongly NUL placement suggests UTF-16.
func nulProfile(raw []byte) (le, be int) {
	if len(raw) < 2 {
		return 0, 0
	}
	for i := 0; i+1 < len(raw); i += 2 {
		if raw[i+1] == 0 && raw[i] != 0 {
			le++
		}
		if raw[i] == 0 && raw[i+1] != 0 {
			be++
		}
	}
	return
}

func hanRatio(b []byte) float64 {
	var han, total int
	for _, r := range string(b) {
		if r == utf8.RuneError {
			continue
		}
		total++
		if (r >= 0x4E00 && r <= 0x9FFF) || (r >= 0x3400 && r <= 0x4DBF) {
			han++
		}
	}
	if total == 0 {
		return 0
	}
	return float64(han) / float64(total)
}

func (r *Result) countArtifacts() {
	for _, c := range r.Text {
		switch c {
		case '\uFFFD':
			r.HasReplacement = true
			r.ReplacementCount++
		case 0:
			r.NulCount++
		}
	}
}

// detectMojibake flags double-encoded UTF-8: CJK text that was encoded as
// UTF-8 a second time appears as characteristic Latin-1 mojibake prefixes.
func (r *Result) detectMojibake() {
	hits := 0
	for i := 0; i+1 < len(r.Text); i++ {
		if r.Text[i] == 0xC3 && r.Text[i+1] >= 0x80 && r.Text[i+1] <= 0xBF {
			hits++
		}
		if r.Text[i] == 0xC2 && r.Text[i+1] >= 0x80 && r.Text[i+1] <= 0xBF {
			hits++
		}
	}
	if hits >= 3 {
		r.Mojibake = true
		r.NotesList = append(r.NotesList, "double-encoded UTF-8 (mojibake) detected; decode as Latin-1 then UTF-8 to recover")
	}
}
