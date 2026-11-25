package aache

import (
	"testing"
	"time"
)

func TestOutputSampleMapping(t *testing.T) {
	t.Parallel()

	timeline, err := BuildTimeline(Stream{
		CoreRate: 24000,
		Frames: []Frame{
			{Config: &ExtensionUpdate{SBR: sbr(48000)}},
			{},
			{},
		},
	})
	if err != nil {
		t.Fatalf("BuildTimeline: %v", err)
	}

	frame, ok := timeline.FrameAtOutputSample(2048)
	if !ok || frame.Index != 1 {
		t.Fatalf("sample 2048: got %+v, %v", frame, ok)
	}
	at, ok := timeline.TimeAtOutputSample(2049)
	if !ok {
		t.Fatal("sample 2049 not mapped")
	}
	if want := time.Duration(42687500); at != want {
		t.Fatalf("sample 2049 time: got %s, want %s", at, want)
	}
	if _, ok := timeline.FrameAtOutputSample(timeline.OutputSamples); ok {
		t.Fatal("end position should be outside the timeline")
	}

	frame, ok = timeline.FrameAtTime(time.Duration(42687500))
	if !ok || frame.Index != 1 {
		t.Fatalf("time lookup: got %+v, %v", frame, ok)
	}
	if _, ok := timeline.FrameAtTime(timeline.Duration); ok {
		t.Fatal("end time should be outside the timeline")
	}
}

func TestMappingAcrossOutputRateChange(t *testing.T) {
	t.Parallel()

	timeline, err := BuildTimeline(Stream{
		CoreRate: 24000,
		Frames: []Frame{
			{},
			{Config: &ExtensionUpdate{SBR: sbr(48000)}},
			{},
		},
	})
	if err != nil {
		t.Fatalf("BuildTimeline: %v", err)
	}
	if timeline.Frames[0].OutputSamples != 1024 || timeline.Frames[1].OutputSamples != 2048 {
		t.Fatalf("unexpected output counts: %+v", timeline.Frames)
	}
	at, ok := timeline.TimeAtOutputSample(1024)
	if !ok {
		t.Fatal("boundary sample not mapped")
	}
	if want := timeline.Frames[1].Start; at != want {
		t.Fatalf("boundary time: got %s, want %s", at, want)
	}
}
