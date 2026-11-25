package mp3timeline

import (
	"encoding/binary"
	"strings"
)

// XingHeader describes a Xing, Info or VBRI VBR header.
type XingHeader struct {
	Kind        string `json:"kind"`
	Offset      int64  `json:"offset"`
	HasFrames   bool   `json:"has_frames"`
	FrameCount  uint32 `json:"frame_count"`
	HasBytes    bool   `json:"has_bytes"`
	StreamBytes uint32 `json:"stream_bytes"`
	HasTOC      bool   `json:"has_toc"`
	TOCEntries  int    `json:"toc_entries"`
	HasQuality  bool   `json:"has_quality"`
	Quality     uint32 `json:"quality"`
}

// GaplessInfo is the LAME/Lavf extension stored after a Xing header.
type GaplessInfo struct {
	Present      bool   `json:"present"`
	Encoder      string `json:"encoder"`
	TagOffset    int64  `json:"tag_offset"`
	EncoderDelay int    `json:"encoder_delay"`
	TailPadding  int    `json:"tail_padding"`
}

const xingTOCEntries = 100

func findVBRHeader(frame []byte, header mpegHeader) (kind string, rel int, ok bool) {
	expected := header.xingOffset()
	if markerAt(frame, expected) {
		return string(frame[expected : expected+4]), expected, true
	}
	withoutCRC := 4 + header.sideInfoSize
	if withoutCRC != expected && markerAt(frame, withoutCRC) {
		return string(frame[withoutCRC : withoutCRC+4]), withoutCRC, true
	}
	if len(frame) >= 40 && string(frame[36:40]) == "VBRI" {
		return "VBRI", 36, true
	}

	start := expected
	if start < 4 {
		start = 4
	}
	end := len(frame)
	if end > start+1024 {
		end = start + 1024
	}
	for i := start; i+4 <= end; i++ {
		if markerAt(frame, i) {
			return string(frame[i : i+4]), i, true
		}
	}
	return "", 0, false
}

func markerAt(frame []byte, offset int) bool {
	if offset < 0 || offset+4 > len(frame) {
		return false
	}
	m := string(frame[offset : offset+4])
	return m == "Xing" || m == "Info"
}

func parseXingAt(frame []byte, rel int, absolute int64) (*XingHeader, *GaplessInfo) {
	kind := string(frame[rel : rel+4])
	if kind == "VBRI" {
		return parseVBRI(frame, rel, absolute), nil
	}

	x := &XingHeader{Kind: kind, Offset: absolute}
	pos := rel + 4
	if pos+4 > len(frame) {
		return x, nil
	}
	flags := binary.BigEndian.Uint32(frame[pos : pos+4])
	pos += 4

	if flags&0x01 != 0 && pos+4 <= len(frame) {
		x.HasFrames = true
		x.FrameCount = binary.BigEndian.Uint32(frame[pos : pos+4])
		pos += 4
	}
	if flags&0x02 != 0 && pos+4 <= len(frame) {
		x.HasBytes = true
		x.StreamBytes = binary.BigEndian.Uint32(frame[pos : pos+4])
		pos += 4
	}
	if flags&0x04 != 0 && pos+xingTOCEntries <= len(frame) {
		x.HasTOC = true
		x.TOCEntries = xingTOCEntries
		pos += xingTOCEntries
	}
	if flags&0x08 != 0 && pos+4 <= len(frame) {
		x.HasQuality = true
		x.Quality = binary.BigEndian.Uint32(frame[pos : pos+4])
		pos += 4
	}

	return x, parseLAMEExtension(frame, pos, absolute)
}

func parseLAMEExtension(frame []byte, pos int, xingOffset int64) *GaplessInfo {
	if pos+24 > len(frame) {
		return nil
	}
	sig := string(frame[pos : pos+4])
	switch sig {
	case "LAME", "Lavc", "Lavf", "L3.99", "GOGO":
	default:
		return nil
	}

	encoder := strings.TrimRight(string(frame[pos:pos+9]), "\x00 ")
	packed := uint32(frame[pos+21])<<16 | uint32(frame[pos+22])<<8 | uint32(frame[pos+23])
	return &GaplessInfo{
		Present:      true,
		Encoder:      encoder,
		TagOffset:    xingOffset + int64(pos),
		EncoderDelay: int(packed >> 12),
		TailPadding:  int(packed & 0x0fff),
	}
}

func parseVBRI(frame []byte, rel int, absolute int64) *XingHeader {
	x := &XingHeader{Kind: "VBRI", Offset: absolute}
	if rel+18 > len(frame) {
		return x
	}
	if binary.BigEndian.Uint16(frame[rel+4:rel+6]) != 1 {
		return x
	}
	x.HasBytes = true
	x.StreamBytes = binary.BigEndian.Uint32(frame[rel+10 : rel+14])
	x.HasFrames = true
	x.FrameCount = binary.BigEndian.Uint32(frame[rel+14 : rel+18])
	return x
}
