package cafmarker

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestPrimingAndRemainderCoordinates(t *testing.T) {
	sizes := []int64{100, 120}
	pakt := paktPayload(2, 1248, 100, 700, sizes...)
	file := cafFile(
		chunk("desc", descPayload(48000, "aac ", 0, 0, 1024, 2, 0)),
		chunk("pakt", pakt),
		dataChunk(220, make([]byte, 220)),
	)
	f := parseBytes(t, file)
	m, err := f.BuildMap()
	if err != nil {
		t.Fatal(err)
	}
	if m.AudibleFrameCount != 1248 || len(m.Issues) != 0 {
		t.Fatalf("audible=%d issues=%v, want 1248/no issues", m.AudibleFrameCount, m.Issues)
	}
	priming := m.LocateFrame(50, f)
	if !contains(priming.Status, "priming_frame") || priming.AudibleFrame != 0 {
		t.Fatalf("priming location=%+v", priming)
	}
	audible := m.LocateFrame(1000, f)
	if audible.AudibleFrame != 900 {
		t.Fatalf("audible frame=%d, want 900", audible.AudibleFrame)
	}
	remainder := m.LocateFrame(1500, f)
	if !contains(remainder.Status, "remainder_frame") {
		t.Fatalf("remainder location=%+v", remainder)
	}
}

func TestUnknownSizeDataChunkExtendsToEOF(t *testing.T) {
	audio := make([]byte, 64)
	var data bytes.Buffer
	data.WriteString("data")
	_ = binary.Write(&data, binary.BigEndian, int64(-1))
	_ = binary.Write(&data, binary.BigEndian, uint32(3))
	data.Write(audio)

	file := cafFile(
		chunk("desc", descPayload(48000, "lpcm", 0, 4, 1, 2, 16)),
		data.Bytes(),
	)
	f := parseBytes(t, file)
	if f.Data == nil || !f.Data.UnknownSize || f.Data.EditCount != 3 {
		t.Fatalf("data chunk parse wrong: %+v", f.Data)
	}
	if f.Data.AudioSize != 64 || f.Data.AvailableAudioSize != 64 {
		t.Fatalf("unknown data size=%d available=%d, want 64", f.Data.AudioSize, f.Data.AvailableAudioSize)
	}
	m, err := f.BuildMap()
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Packets) != 16 {
		t.Fatalf("packets=%d, want 16 stereo 16-bit frames", len(m.Packets))
	}
}

func TestVariableBytesAndFramesPairs(t *testing.T) {
	values := []int64{11, 512, 22, 400}
	file := cafFile(
		chunk("desc", descPayload(48000, "vorb", 0, 0, 0, 2, 0)),
		chunk("pakt", paktPayload(2, 912, 0, 0, values...)),
		dataChunk(33, make([]byte, 33)),
	)
	f := parseBytes(t, file)
	m, err := f.BuildMap()
	if err != nil {
		t.Fatal(err)
	}
	if m.Packets[0].ByteSize != 11 || m.Packets[0].FrameCount != 512 {
		t.Fatalf("packet0=%+v", m.Packets[0])
	}
	if m.Packets[1].ByteOffset != 11 || m.Packets[1].FrameStart != 512 || m.Packets[1].FrameCount != 400 {
		t.Fatalf("packet1=%+v", m.Packets[1])
	}
}

func TestLargeConstantLayoutIsArithmetic(t *testing.T) {
	const packets = 20000
	audio := make([]byte, packets*4)
	file := cafFile(
		chunk("desc", descPayload(48000, "lpcm", 0, 4, 1, 2, 16)),
		dataChunk(int64(len(audio)), audio),
	)
	f := parseBytes(t, file)
	m, err := f.BuildMap()
	if err != nil {
		t.Fatal(err)
	}
	if m.Constant == nil || m.PacketCountMapped != packets || len(m.Packets) != 0 || !m.PacketsTruncated {
		t.Fatalf("map should stay compact: constant=%+v count=%d len=%d", m.Constant, m.PacketCountMapped, len(m.Packets))
	}
	p, ok := m.PacketAt(12345)
	if !ok || p.FrameStart != 12345 || p.ByteOffset != 49380 {
		t.Fatalf("PacketAt(12345)=%+v ok=%v", p, ok)
	}
	loc := m.LocateFrame(12345, f)
	if loc.PacketIndex != 12345 || loc.AudioByteOffset != 49380 {
		t.Fatalf("compact location=%+v", loc)
	}
}
