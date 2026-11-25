package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
)

// Tag carries the metadata written into the trailing ID3v2.4 tag.
type Tag struct {
	Title, Artist, Album, Track, Genre, Date string
	Chapters                                 []ChapterTime // resolved chapter timestamps in milliseconds
}

// ChapterTime is a chapter with absolute millisecond bounds, ready for
// ID3 CHAP frames.
type ChapterTime struct {
	Title          string
	StartMs, EndMs uint32
}

// synchsafe encodes a 28-bit value as 4 sync-safe bytes (ID3v2.4 sizes).
func synchsafe(v uint32) []byte {
	return []byte{
		byte(v >> 21 & 0x7f),
		byte(v >> 14 & 0x7f),
		byte(v >> 7 & 0x7f),
		byte(v & 0x7f),
	}
}

// textFrame builds a UTF-8 text frame (encoding byte 0x03).
func textFrame(id, value string) []byte {
	body := append([]byte{0x03}, []byte(value)...)
	return frame(id, body)
}

func frame(id string, body []byte) []byte {
	var b bytes.Buffer
	b.WriteString(id)
	b.Write(synchsafe(uint32(len(body))))
	b.Write([]byte{0x00, 0x00}) // flags
	b.Write(body)
	return b.Bytes()
}

// chapFrame builds a CHAP frame: element ID, start/end ms, byte offsets
// flagged as unknown (0xFFFFFFFF), then a nested TIT2 sub-frame.
func chapFrame(idx int, c ChapterTime) []byte {
	var body bytes.Buffer
	fmt.Fprintf(&body, "ch%02d", idx)
	body.WriteByte(0x00)
	binary.Write(&body, binary.BigEndian, c.StartMs)
	binary.Write(&body, binary.BigEndian, c.EndMs)
	body.Write([]byte{0xff, 0xff, 0xff, 0xff})
	body.Write([]byte{0xff, 0xff, 0xff, 0xff})
	body.Write(textFrame("TIT2", c.Title))
	return frame("CHAP", body.Bytes())
}

// ctocFrame builds a CTOC frame referencing every chapter element ID.
func ctocFrame(n int) []byte {
	var body bytes.Buffer
	body.WriteString("toc")
	body.WriteByte(0x00)
	body.WriteByte(0x03) // top-level + ordered
	body.WriteByte(byte(n))
	for i := 0; i < n; i++ {
		fmt.Fprintf(&body, "ch%02d", i)
		body.WriteByte(0x00)
	}
	return frame("CTOC", body.Bytes())
}

// BuildID3 serializes the tag as a complete ID3v2.4 tag (header+frames).
func BuildID3(t *Tag) []byte {
	var frames bytes.Buffer
	add := func(id, v string) {
		if strings.TrimSpace(v) != "" {
			frames.Write(textFrame(id, v))
		}
	}
	add("TIT2", t.Title)
	add("TPE1", t.Artist)
	add("TALB", t.Album)
	add("TRCK", t.Track)
	add("TCON", t.Genre)
	add("TDRC", t.Date)
	if len(t.Chapters) > 0 {
		frames.Write(ctocFrame(len(t.Chapters)))
		for i, c := range t.Chapters {
			frames.Write(chapFrame(i, c))
		}
	}
	var tag bytes.Buffer
	tag.WriteString("ID3")
	tag.Write([]byte{0x04, 0x00, 0x00}) // v2.4.0, no flags
	tag.Write(synchsafe(uint32(frames.Len())))
	tag.Write(frames.Bytes())
	return tag.Bytes()
}
