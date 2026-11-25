// genfixtures regenerates the encoded subtitle variants in testdata from the
// canonical UTF-8 lecture.srt. It exists so fixtures are reproducible without
// iconv or platform-specific shell pipelines.
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/text/encoding/simplifiedchinese"
)

func main() {
	root := os.Args[1]
	dir := filepath.Join(root, "testdata", "subtitles")
	raw, err := os.ReadFile(filepath.Join(dir, "lecture.srt"))
	must(err)

	must(os.WriteFile(filepath.Join(dir, "lecture.utf16le.srt"),
		withBOM(binary.LittleEndian, utf16Encode(raw, false)), 0o644))
	must(os.WriteFile(filepath.Join(dir, "lecture.utf16be.srt"),
		withBOM(binary.BigEndian, utf16Encode(raw, true)), 0o644))

	gb, err := simplifiedchinese.GB18030.NewEncoder().Bytes(raw)
	must(err)
	must(os.WriteFile(filepath.Join(dir, "lecture.gb18030.srt"), gb, 0o644))

	// Double encode: each original UTF-8 byte is reinterpreted as a Latin-1
	// code point and then encoded as UTF-8 again. A byte 0xE4 therefore
	// becomes the two-byte UTF-8 sequence for U+00E4 (ä), the canonical
	// "UTF-8 saved twice" mojibake signature.
	var moj bytes.Buffer
	for _, b := range raw {
		moj.WriteRune(rune(b))
	}
	must(os.WriteFile(filepath.Join(dir, "lecture.mojibake.srt"), moj.Bytes(), 0o644))

	fontDir := filepath.Join(root, "testdata", "fonts")
	must(buildTTC(
		filepath.Join(fontDir, "collection.ttc"),
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
		"/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf",
	))

	fmt.Println("fixtures regenerated in", dir)
}

// buildTTC packs standalone SFNT fonts into a version 1.0 TTC. The layout is
// the 12-byte 'ttcf' header, offset table, then each font verbatim at a
// 4-byte aligned offset. Each embedded font's table records are relocated to
// absolute file offsets, which is what sfnt.ParseCollection expects.
func buildTTC(out string, fonts ...string) error {
	blobs := make([][]byte, 0, len(fonts))
	for _, p := range fonts {
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		blobs = append(blobs, b)
	}
	headerSize := 12 + 4*len(blobs)
	offsets := make([]uint32, len(blobs))
	var body bytes.Buffer
	for i, b := range blobs {
		base := uint32(headerSize + body.Len())
		offsets[i] = base
		relocated, err := relocateFont(b, base)
		if err != nil {
			return err
		}
		body.Write(relocated)
		for body.Len()%4 != 0 {
			body.WriteByte(0)
		}
	}
	var hdr bytes.Buffer
	hdr.Write([]byte("ttcf"))
	hdr.Write([]byte{0x00, 0x01, 0x00, 0x00}) // TTC version 1.0
	tmp := make([]byte, 4)
	binary.BigEndian.PutUint32(tmp, uint32(len(blobs)))
	hdr.Write(tmp)
	for _, off := range offsets {
		binary.BigEndian.PutUint32(tmp, off)
		hdr.Write(tmp)
	}
	return os.WriteFile(out, append(hdr.Bytes(), body.Bytes()...), 0o644)
}

// relocateFont rewrites the 4-byte table offset field (bytes 8..11) in every
// table record so it points at the absolute file position inside the TTC.
func relocateFont(font []byte, base uint32) ([]byte, error) {
	if len(font) < 12 {
		return nil, fmt.Errorf("font too short for SFNT header")
	}
	numTables := int(binary.BigEndian.Uint16(font[4:6]))
	out := append([]byte(nil), font...)
	for i := 0; i < numTables; i++ {
		rec := 12 + i*16
		if rec+16 > len(out) {
			return nil, fmt.Errorf("table record %d out of bounds", i)
		}
		tableOffset := binary.BigEndian.Uint32(out[rec+8 : rec+12])
		binary.BigEndian.PutUint32(out[rec+8:rec+12], base+tableOffset)
	}
	return out, nil
}

func utf16Encode(b []byte, big bool) []byte {
	runes := []rune(string(b))
	out := make([]byte, 0, len(runes)*2)
	var buf [2]byte
	for _, r := range runes {
		if big {
			binary.BigEndian.PutUint16(buf[:], uint16(r))
		} else {
			binary.LittleEndian.PutUint16(buf[:], uint16(r))
		}
		out = append(out, buf[0], buf[1])
	}
	return out
}

func withBOM(order binary.ByteOrder, body []byte) []byte {
	out := make([]byte, 2+len(body))
	if order == binary.BigEndian {
		out[0], out[1] = 0xFE, 0xFF
	} else {
		out[0], out[1] = 0xFF, 0xFE
	}
	copy(out[2:], body)
	return out
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "genfixtures:", err)
		os.Exit(1)
	}
}
