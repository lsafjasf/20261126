package bwf

import (
	"math"
	"testing"
)

func TestTimecodeAt25FPS(t *testing.T) {
	rate, err := ParseRate("25")
	if err != nil {
		t.Fatal(err)
	}
	tc, err := ParseTimecode("10:00:00:00", "25")
	if err != nil {
		t.Fatal(err)
	}
	frame, err := tc.FrameNumber(rate)
	if err != nil {
		t.Fatal(err)
	}
	if frame != 900000 {
		t.Fatalf("frame=%d, want 900000", frame)
	}
	samples, err := TimecodeSample(tc, rate, 48000)
	if err != nil {
		t.Fatal(err)
	}
	if samples != 1728000000 {
		t.Fatalf("samples=%d, want 1728000000", samples)
	}
}

func TestDropFrameMapsLabelsToActualFrames(t *testing.T) {
	rate, err := ParseRate("29.97")
	if err != nil {
		t.Fatal(err)
	}
	tc, err := ParseTimecode("00:01:00;02", "29.97")
	if err != nil {
		t.Fatal(err)
	}
	frame, err := tc.FrameNumber(rate)
	if err != nil {
		t.Fatal(err)
	}
	if frame != 1800 {
		t.Fatalf("frame=%d, want 1800", frame)
	}
	samples, err := TimecodeSample(tc, rate, 48000)
	if err != nil {
		t.Fatal(err)
	}
	if samples != 2882880 {
		t.Fatalf("samples=%d, want 2882880", samples)
	}
	if _, err := ParseTimecode("00:01:00;00", "29.97"); err == nil {
		t.Fatal("dropped label 00:01:00;00 should be rejected")
	}
}

func TestFrameSamplesIsRational(t *testing.T) {
	rate, err := ParseRate("25")
	if err != nil {
		t.Fatal(err)
	}
	if got := FrameSamples(rate, 48000); math.Abs(got-1920) > 1e-9 {
		t.Fatalf("frame samples=%f, want 1920", got)
	}
	rate, err = ParseRate("30000/1001")
	if err != nil {
		t.Fatal(err)
	}
	if got := FrameSamples(rate, 48000); math.Abs(got-1601.6) > 1e-9 {
		t.Fatalf("frame samples=%f, want 1601.6", got)
	}
}

func TestSampleClock(t *testing.T) {
	if got := sampleClock(1728000384, 48000); got != "10:00:00.008000 (1728000384 samples)" {
		t.Fatalf("sampleClock=%q", got)
	}
}
