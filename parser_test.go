package adts

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseBytesSingleSegment(t *testing.T) {
	first := testFrame(0, 1, 4, 2, 0, 16)
	first[7], first[8] = 0xff, 0xf1 // sync-like bytes in the payload must not split the frame
	stream := bytes.Join([][]byte{
		first,
		testFrame(0, 1, 4, 2, 0, 10),
		testFrame(0, 1, 4, 2, 0, 10),
	}, nil)

	report := ParseBytes(stream)
	if len(report.BadFrames) != 0 {
		t.Fatalf("BadFrames = %+v, want none", report.BadFrames)
	}
	if len(report.Segments) != 1 {
		t.Fatalf("Segments = %d, want 1", len(report.Segments))
	}
	if report.FrameCount != 3 {
		t.Fatalf("FrameCount = %d, want 3", report.FrameCount)
	}
	if report.TotalSamples != 3*SamplesPerRawDataBlock {
		t.Fatalf("TotalSamples = %d, want %d", report.TotalSamples, 3*SamplesPerRawDataBlock)
	}
	segment := report.Segments[0]
	if segment.StartOffset != 0 || segment.EndOffset != int64(len(stream)) {
		t.Fatalf("segment offsets = [%d,%d], want [0,%d]",
			segment.StartOffset, segment.EndOffset, len(stream))
	}
	if segment.SampleCount != 3*SamplesPerRawDataBlock {
		t.Fatalf("SampleCount = %d, want %d", segment.SampleCount, 3*SamplesPerRawDataBlock)
	}
	if segment.CumulativeSamples != segment.SampleCount {
		t.Fatalf("CumulativeSamples = %d, want %d", segment.CumulativeSamples, segment.SampleCount)
	}
}

func TestParseBytesSplitsOnStateChanges(t *testing.T) {
	frames := [][]byte{
		testFrame(0, 1, 4, 2, 0, 4), // MPEG-4 LC 44.1 kHz stereo
		testFrame(0, 1, 4, 2, 0, 4),
		testFrame(0, 1, 3, 2, 0, 4), // sample rate switch to 48 kHz
		testFrame(0, 0, 4, 2, 0, 4), // profile switch to Main at 44.1 kHz
		testFrame(0, 0, 4, 2, 0, 4),
	}
	stream := bytes.Join(frames, nil)

	report := ParseBytes(stream)
	if len(report.BadFrames) != 0 {
		t.Fatalf("BadFrames = %+v, want none", report.BadFrames)
	}
	if len(report.Segments) != 3 {
		t.Fatalf("Segments = %d, want 3", len(report.Segments))
	}

	wantFrames := []int{2, 1, 2}
	wantSamples := []int64{2048, 1024, 2048}
	wantCumulative := []int64{2048, 3072, 5120}
	for i, segment := range report.Segments {
		if segment.FrameCount != wantFrames[i] {
			t.Errorf("segment %d FrameCount = %d, want %d", i, segment.FrameCount, wantFrames[i])
		}
		if segment.SampleCount != wantSamples[i] {
			t.Errorf("segment %d SampleCount = %d, want %d", i, segment.SampleCount, wantSamples[i])
		}
		if segment.CumulativeSamples != wantCumulative[i] {
			t.Errorf("segment %d CumulativeSamples = %d, want %d",
				i, segment.CumulativeSamples, wantCumulative[i])
		}
	}
	if report.Segments[1].State.SampleRate != 48000 {
		t.Errorf("segment 1 SampleRate = %d, want 48000", report.Segments[1].State.SampleRate)
	}
	if report.Segments[2].State.Profile != 0 {
		t.Errorf("segment 2 Profile = %d, want 0", report.Segments[2].State.Profile)
	}
	if report.TotalSamples != 5120 {
		t.Errorf("TotalSamples = %d, want 5120", report.TotalSamples)
	}
}

func TestParseBytesSeparatesBadRegions(t *testing.T) {
	first := testFrame(0, 1, 4, 2, 0, 6)
	second := testFrame(0, 1, 4, 2, 0, 6)
	stream := bytes.Join([][]byte{
		{0x00, 0x01, 0x02},
		first,
		{0x03, 0x04, 0x05, 0x06},
		second,
	}, nil)

	report := ParseBytes(stream)
	if len(report.BadFrames) != 2 {
		t.Fatalf("BadFrames = %+v, want 2", report.BadFrames)
	}
	if report.BadFrames[0].Offset != 0 || report.BadFrames[0].Length != 3 {
		t.Errorf("BadFrames[0] = %+v, want offset 0 length 3", report.BadFrames[0])
	}
	if report.BadFrames[1].Offset != int64(3+len(first)) ||
		report.BadFrames[1].Length != 4 {
		t.Errorf("BadFrames[1] = %+v, want offset %d length 4",
			report.BadFrames[1], 3+len(first))
	}
	if len(report.Segments) != 2 {
		t.Fatalf("Segments = %d, want 2", len(report.Segments))
	}
	if report.TotalSamples != 2*SamplesPerRawDataBlock {
		t.Errorf("TotalSamples = %d, want %d", report.TotalSamples, 2*SamplesPerRawDataBlock)
	}
	if report.Segments[1].CumulativeSamples != 2*SamplesPerRawDataBlock {
		t.Errorf("CumulativeSamples = %d, want %d",
			report.Segments[1].CumulativeSamples, 2*SamplesPerRawDataBlock)
	}
}

func TestParseBytesRecoversFromTruncatedFrameLength(t *testing.T) {
	first := testFrame(0, 1, 4, 2, 0, 6)
	broken := testFrameWithDeclaredLength(0, 1, 4, 2, 0, 500, 2)
	second := testFrame(0, 0, 3, 2, 0, 6)
	stream := bytes.Join([][]byte{first, broken, second}, nil)

	report := ParseBytes(stream)
	if len(report.BadFrames) != 1 {
		t.Fatalf("BadFrames = %+v, want 1", report.BadFrames)
	}
	if !strings.Contains(report.BadFrames[0].Reason, ErrTruncatedFrame.Error()) {
		t.Errorf("reason = %q, want %q", report.BadFrames[0].Reason, ErrTruncatedFrame)
	}
	if len(report.Segments) != 2 {
		t.Fatalf("Segments = %d, want 2", len(report.Segments))
	}
	if report.FrameCount != 2 || report.TotalSamples != 2*SamplesPerRawDataBlock {
		t.Errorf("FrameCount/TotalSamples = %d/%d, want 2/%d",
			report.FrameCount, report.TotalSamples, 2*SamplesPerRawDataBlock)
	}
}

func TestParseBytesRecoversFromOverlappingFrameLength(t *testing.T) {
	first := testFrame(0, 1, 4, 2, 0, 6)
	broken := testFrameWithDeclaredLength(0, 1, 4, 2, 0, 40, 10)
	second := testFrame(0, 0, 3, 2, 0, 20)
	stream := bytes.Join([][]byte{first, broken, second}, nil)

	report := ParseBytes(stream)
	if len(report.BadFrames) != 1 {
		t.Fatalf("BadFrames = %+v, want 1", report.BadFrames)
	}
	if !strings.Contains(report.BadFrames[0].Reason, ErrFrameOverlap.Error()) {
		t.Errorf("reason = %q, want %q", report.BadFrames[0].Reason, ErrFrameOverlap)
	}
	if len(report.Segments) != 2 {
		t.Fatalf("Segments = %d, want 2", len(report.Segments))
	}
	if report.FrameCount != 2 || report.TotalSamples != 2*SamplesPerRawDataBlock {
		t.Errorf("FrameCount/TotalSamples = %d/%d, want 2/%d",
			report.FrameCount, report.TotalSamples, 2*SamplesPerRawDataBlock)
	}
}

func TestParseBytesCountsRawDataBlocks(t *testing.T) {
	stream := testFrame(0, 1, 4, 2, 2, 8)

	report := ParseBytes(stream)
	if report.FrameCount != 1 {
		t.Fatalf("FrameCount = %d, want 1", report.FrameCount)
	}
	if report.TotalSamples != 3*SamplesPerRawDataBlock {
		t.Fatalf("TotalSamples = %d, want %d",
			report.TotalSamples, 3*SamplesPerRawDataBlock)
	}
}

func TestParseBytesRecordsTruncatedTail(t *testing.T) {
	frame := testFrame(0, 1, 4, 2, 0, 5)
	stream := append(append([]byte(nil), frame...), 0xff, 0xf1)

	report := ParseBytes(stream)
	if len(report.BadFrames) != 1 {
		t.Fatalf("BadFrames = %+v, want 1", report.BadFrames)
	}
	if report.BadFrames[0].Offset != int64(len(frame)) ||
		report.BadFrames[0].Length != 2 {
		t.Errorf("BadFrames[0] = %+v, want offset %d length 2",
			report.BadFrames[0], len(frame))
	}
	if report.TotalSamples != SamplesPerRawDataBlock {
		t.Errorf("TotalSamples = %d, want %d", report.TotalSamples, SamplesPerRawDataBlock)
	}
}

func testFrame(mpegID, profile, sampleRateIndex, channelConfig, rawBlocks, payloadSize int) []byte {
	length := FrameHeaderSize + payloadSize
	return testFrameWithDeclaredLength(
		mpegID, profile, sampleRateIndex, channelConfig, rawBlocks, length, payloadSize,
	)
}

func testFrameWithDeclaredLength(mpegID, profile, sampleRateIndex, channelConfig, rawBlocks, declaredLength, payloadSize int) []byte {
	frame := make([]byte, FrameHeaderSize+payloadSize)
	frame[0] = 0xff
	frame[1] = 0xf0 | byte(mpegID&0x01)<<3 | 0x01 // layer 0, no CRC
	frame[2] = byte(profile&0x03)<<6 | byte(sampleRateIndex&0x0f)<<2 |
		byte(channelConfig>>2)&0x01
	frame[3] = byte(channelConfig&0x03)<<6 | byte(declaredLength>>11)&0x03
	frame[4] = byte(declaredLength >> 3)
	frame[5] = byte(declaredLength&0x07)<<5 | 0x1f
	frame[6] = 0xfc | byte(rawBlocks&0x03)
	for i := FrameHeaderSize; i < len(frame); i++ {
		frame[i] = 0xaa
	}
	return frame
}
