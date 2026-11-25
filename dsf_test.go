package main

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

const (
	dsd64  = 2822400
	dsd128 = 5644800
)

// buildDSF synthesizes a stereo DSF: DSD64 segment + DSD128 segment,
// with a trailing partial block (tail not a multiple of block*channels).
func buildDSF(t *testing.T) (path string, payload []byte, seg1, seg2 uint64) {
	t.Helper()
	block := 4096
	ch := 2
	// Segment 1: 3 full blocks at DSD64 -> 3*4096*8 samples per channel.
	// Segment 2: 2 full blocks + 100 tail bytes/channel at DSD128.
	seg1 = 3 * uint64(block) * 8
	seg2 = 2*uint64(block)*8 + 100*8
	dataBytes := (3+2)*block*ch + 100*ch

	payload = make([]byte, dataBytes)
	for i := range payload {
		payload[i] = byte(i*31 + 7) // deterministic non-zero pattern
	}

	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, [4]byte{'D', 'S', 'D', ' '})
	binary.Write(&buf, binary.LittleEndian, uint64(28))
	binary.Write(&buf, binary.LittleEndian, uint64(0)) // file size, patched below
	binary.Write(&buf, binary.LittleEndian, uint64(0)) // metadata offset: none

	binary.Write(&buf, binary.LittleEndian, [4]byte{'f', 'm', 't', ' '})
	binary.Write(&buf, binary.LittleEndian, uint64(52))
	binary.Write(&buf, binary.LittleEndian, uint32(1))     // version
	binary.Write(&buf, binary.LittleEndian, uint32(0))     // format id
	binary.Write(&buf, binary.LittleEndian, uint32(2))     // channel type: stereo
	binary.Write(&buf, binary.LittleEndian, uint32(ch))    // channels
	binary.Write(&buf, binary.LittleEndian, uint32(dsd64)) // header rate
	binary.Write(&buf, binary.LittleEndian, uint32(1))     // bits per sample
	binary.Write(&buf, binary.LittleEndian, seg1+seg2)     // sample count
	binary.Write(&buf, binary.LittleEndian, uint32(block)) // block per channel
	binary.Write(&buf, binary.LittleEndian, uint32(0))     // reserved

	binary.Write(&buf, binary.LittleEndian, [4]byte{'d', 'a', 't', 'a'})
	binary.Write(&buf, binary.LittleEndian, uint64(12+dataBytes))
	buf.Write(payload)

	raw := buf.Bytes()
	binary.LittleEndian.PutUint64(raw[12:20], uint64(len(raw)))

	path = filepath.Join(t.TempDir(), "spliced.dsf")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return path, payload, seg1, seg2
}

func TestParseAndLayout(t *testing.T) {
	path, _, seg1, seg2 := buildDSF(t)
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	st, _ := f.Stat()
	info, err := Parse(f, st.Size())
	if err != nil {
		t.Fatal(err)
	}
	if info.SampleFreq != dsd64 || info.ChannelNum != 2 || info.BlockSizePerChannel != 4096 {
		t.Fatalf("fmt fields wrong: %+v", info)
	}
	if info.SampleCount != seg1+seg2 {
		t.Fatalf("sample count %d, want %d", info.SampleCount, seg1+seg2)
	}
	l := info.Layout()
	if l.FullBlocks != 5 || l.PartialPerChannel != 100 || l.StrayBytes != 0 {
		t.Fatalf("layout wrong: %+v", l)
	}
	if got := info.ActualSampleCount(); got != seg1+seg2 {
		t.Fatalf("actual samples %d, want %d", got, seg1+seg2)
	}
}

func TestTimelineMixedRates(t *testing.T) {
	_, _, seg1, seg2 := buildDSF(t)
	tl, err := NewTimeline([]Segment{
		{Samples: seg1, Rate: dsd64},
		{Samples: seg2, Rate: dsd128},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := tl.TimeAt(0); got != 0 {
		t.Fatalf("TimeAt(0) = %d", got)
	}
	// Boundary sample starts exactly at segment 1's duration.
	if got, want := tl.TimeAt(seg1), (Segment{seg1, dsd64}).DurationNanos(); got != want {
		t.Fatalf("TimeAt(seg1) = %d, want %d", got, want)
	}
	total, totalN := tl.Totals()
	if total != seg1+seg2 {
		t.Fatalf("total samples %d", total)
	}
	// Round-trip the segment-2 midpoint.
	mid := seg1 + seg2/2
	if got := tl.SampleAt(tl.TimeAt(mid)); got+1 < mid || got > mid+1 {
		t.Fatalf("round trip: got %d, want %d (±1 for integer truncation)", got, mid)
	}
	// The PCM misread: counting every DSD sample at 176400 Hz stretches
	// the axis well past 2x for this DSD64/DSD128 mix.
	naive := NaiveNanos(total, 176400)
	if naive <= 2*totalN {
		t.Fatalf("naive axis %d ns not >2x true %d ns", naive, totalN)
	}
	// Chapter at the splice point must resolve to the true boundary time.
	ch := []Chapter{{Title: "splice", Sample: seg1}}
	start, end := tl.ChapterTime(ch, 0)
	if start != (Segment{seg1, dsd64}).DurationNanos() || end != totalN {
		t.Fatalf("chapter bounds %d..%d", start, end)
	}
}
