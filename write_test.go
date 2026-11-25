package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"os"
	"testing"
)

func TestWriteMetadataKeepsPayload(t *testing.T) {
	path, payload, seg1, seg2 := buildDSF(t)
	rawBefore, _ := os.ReadFile(path)
	wantSum := sha256.Sum256(payload)

	f, _ := os.Open(path)
	st, _ := f.Stat()
	info, err := Parse(f, st.Size())
	f.Close()
	if err != nil {
		t.Fatal(err)
	}

	tl, err := NewTimeline([]Segment{
		{Samples: seg1, Rate: dsd64},
		{Samples: seg2, Rate: dsd128},
	})
	if err != nil {
		t.Fatal(err)
	}
	chapters := []Chapter{
		{Title: "Intro", Sample: 0},
		{Title: "DSD128 part", Sample: seg1},
	}
	tag := &Tag{Title: "Spliced", Artist: "Tester"}
	for i, c := range chapters {
		start, end := tl.ChapterTime(chapters, i)
		tag.Chapters = append(tag.Chapters, ChapterTime{
			Title: c.Title, StartMs: uint32(start / 1e6), EndMs: uint32(end / 1e6),
		})
	}
	blob := BuildID3(tag)
	if !bytes.HasPrefix(blob, []byte("ID3\x04")) {
		t.Fatal("tag missing ID3v2.4 header")
	}
	if !bytes.Contains(blob, []byte("CHAP")) || !bytes.Contains(blob, []byte("CTOC")) {
		t.Fatal("tag missing chapter frames")
	}

	if err := WriteMetadata(path, info, blob); err != nil {
		t.Fatal(err)
	}

	rawAfter, _ := os.ReadFile(path)
	// Payload region must be byte-identical.
	off := int(info.DataOffset)
	gotSum := sha256.Sum256(rawAfter[off : off+len(payload)])
	if gotSum != wantSum {
		t.Fatal("DSD payload changed")
	}
	// fmt chunk and data chunk header untouched.
	if !bytes.Equal(rawBefore[28:off], rawAfter[28:off]) {
		t.Fatal("fmt/data chunk headers changed")
	}
	// Header bookkeeping now points at the tag.
	metaOff := binary.LittleEndian.Uint64(rawAfter[20:28])
	if metaOff != info.DataOffset+info.DataSize {
		t.Fatalf("metadata offset %d", metaOff)
	}
	if decl := binary.LittleEndian.Uint64(rawAfter[12:20]); decl != uint64(len(rawAfter)) {
		t.Fatalf("declared file size %d, actual %d", decl, len(rawAfter))
	}
	if !bytes.HasPrefix(rawAfter[metaOff:], []byte("ID3")) {
		t.Fatal("tag not found at metadata offset")
	}
	// Re-parse the rewritten file.
	f2, _ := os.Open(path)
	st2, _ := f2.Stat()
	if _, err := Parse(f2, st2.Size()); err != nil {
		t.Fatalf("re-parse after write: %v", err)
	}
	f2.Close()
}
