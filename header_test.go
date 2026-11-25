package adts

import (
	"errors"
	"testing"
)

func TestParseHeader(t *testing.T) {
	frame := testFrame(1, 1, 4, 2, 0, 11)

	header, err := ParseHeader(frame)
	if err != nil {
		t.Fatalf("ParseHeader returned error: %v", err)
	}
	if header.MPEGVersion != 2 {
		t.Fatalf("MPEGVersion = %d, want 2", header.MPEGVersion)
	}
	if header.Profile != 1 {
		t.Fatalf("Profile = %d, want 1", header.Profile)
	}
	if header.SampleRate != 44100 {
		t.Fatalf("SampleRate = %d, want 44100", header.SampleRate)
	}
	if header.ChannelConfig != 2 {
		t.Fatalf("ChannelConfig = %d, want 2", header.ChannelConfig)
	}
	if header.FrameLength != len(frame) {
		t.Fatalf("FrameLength = %d, want %d", header.FrameLength, len(frame))
	}
	if header.Samples() != SamplesPerRawDataBlock {
		t.Fatalf("Samples = %d, want %d", header.Samples(), SamplesPerRawDataBlock)
	}
}

func TestParseHeaderRejectsReservedSampleRate(t *testing.T) {
	frame := testFrame(0, 1, 13, 2, 0, 1)
	_, err := ParseHeader(frame)
	if !errors.Is(err, ErrSampleRateIndex) {
		t.Fatalf("error = %v, want ErrSampleRateIndex", err)
	}
}
