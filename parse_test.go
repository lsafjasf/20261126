package eac3

import (
	"testing"
)

type testBitWriter struct {
	bits []byte
	acc  byte
	n    uint
}

func (w *testBitWriter) write(value uint32, n uint) {
	for i := int(n) - 1; i >= 0; i-- {
		bit := byte((value >> i) & 1)
		w.acc = (w.acc << 1) | bit
		w.n++
		if w.n == 8 {
			w.bits = append(w.bits, w.acc)
			w.acc, w.n = 0, 0
		}
	}
}

func (w *testBitWriter) bytes() []byte {
	if w.n != 0 {
		w.acc <<= 8 - w.n
		w.bits = append(w.bits, w.acc)
		w.acc, w.n = 0, 0
	}
	return append([]byte(nil), w.bits...)
}

func testFrameHeader(t *testing.T, typ SubstreamType, sid uint8, frameBytes int, sampleRateCode uint8, lowRateCode *uint8, blockCode *uint8, acmod uint8, lfe bool) []byte {
	t.Helper()
	if frameBytes%2 != 0 || frameBytes < 8 {
		t.Fatalf("frame bytes must be even and >= 8: %d", frameBytes)
	}
	w := &testBitWriter{}
	w.write(uint32(SyncWord), 16)
	w.write(uint32(typ), 2)
	w.write(uint32(sid), 3)
	w.write(uint32(frameBytes/2-1), 11)
	w.write(uint32(sampleRateCode), 2)
	if sampleRateCode == 3 {
		if lowRateCode == nil {
			t.Fatal("lowRateCode required")
		}
		w.write(uint32(*lowRateCode), 2)
	} else {
		if blockCode == nil {
			t.Fatal("blockCode required")
		}
		w.write(uint32(*blockCode), 2)
	}
	w.write(uint32(acmod), 3)
	if lfe {
		w.write(1, 1)
	} else {
		w.write(0, 1)
	}
	return w.bytes()
}

func TestParseHeaderFixedFields(t *testing.T) {
	blockCode := uint8(3)
	raw := testFrameHeader(t, SubstreamIndependent, 0, 1200, 0, nil, &blockCode, 7, true)
	h, err := ParseHeader(raw)
	if err != nil {
		t.Fatal(err)
	}
	if h.Type != SubstreamIndependent || h.SubstreamID != 0 {
		t.Fatalf("unexpected type/id: %s/%d", h.Type, h.SubstreamID)
	}
	if h.Spec.SampleRateHz != 48000 {
		t.Fatalf("sample rate=%d", h.Spec.SampleRateHz)
	}
	if h.Spec.Boundary.BlocksPerFrame != 6 || h.Spec.Boundary.SamplesPerFrame != 1536 {
		t.Fatalf("boundary=%+v", h.Spec.Boundary)
	}
	if h.Spec.Boundary.FrameSizeBytes != 1200 {
		t.Fatalf("frame size=%d", h.Spec.Boundary.FrameSizeBytes)
	}
	if h.Spec.Channel.Channels != 6 || !h.Spec.Channel.LFEOn || h.Spec.Channel.Acmod != 7 {
		t.Fatalf("channel=%+v", h.Spec.Channel)
	}
}

func TestParseHeaderDependentLowSampleRate(t *testing.T) {
	lowCode := uint8(1) // 44.1 kHz table entry / 2 => 22050
	raw := testFrameHeader(t, SubstreamDependent, 3, 700, 3, &lowCode, nil, 2, false)
	h, err := ParseHeader(raw)
	if err != nil {
		t.Fatal(err)
	}
	if h.Type != SubstreamDependent || h.SubstreamID != 3 {
		t.Fatalf("unexpected ref %s/%d", h.Type, h.SubstreamID)
	}
	if h.Spec.SampleRateHz != 22050 {
		t.Fatalf("sample rate=%d", h.Spec.SampleRateHz)
	}
	if h.Spec.Boundary.SamplesPerFrame != 1536 || h.Spec.Boundary.BlocksPerFrame != 6 {
		t.Fatalf("low-rate frames should use six blocks: %+v", h.Spec.Boundary)
	}
	if h.Spec.Channel.Channels != 2 {
		t.Fatalf("channels=%d", h.Spec.Channel.Channels)
	}
}

func TestScanHeadersUsesFrameSizeAndSyncRecovery(t *testing.T) {
	blockCode := uint8(0) // one block, 256 samples
	h1 := testFrameHeader(t, SubstreamIndependent, 0, 64, 0, nil, &blockCode, 2, false)
	f1 := make([]byte, 64)
	copy(f1, h1)

	h2 := testFrameHeader(t, SubstreamDependent, 0, 80, 0, nil, &blockCode, 7, true)
	f2 := make([]byte, 80)
	copy(f2, h2)

	stream := append(append([]byte{0, 1, 2}, f1...), f2...)
	headers, consumed, err := ScanHeaders(stream)
	if err != nil {
		t.Fatal(err)
	}
	if len(headers) != 2 {
		t.Fatalf("headers=%d", len(headers))
	}
	if headers[0].ByteOffset != 3 || headers[1].ByteOffset != 67 {
		t.Fatalf("offsets=%d,%d", headers[0].ByteOffset, headers[1].ByteOffset)
	}
	if consumed != len(stream) {
		t.Fatalf("consumed=%d want %d", consumed, len(stream))
	}
}
