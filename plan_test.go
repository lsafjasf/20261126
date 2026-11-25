package eac3

import "testing"

func TestPlanSpecChangeMapsAllFourTriggers(t *testing.T) {
	from := ProgramSpec{
		SampleRateHz: 48000,
		Channel:      ChannelStructure{Acmod: 7, LFEOn: true, Channels: 6},
		DRC:          DRCState{ConfigID: "line-v1"},
		Boundary:     BoundaryState{BlocksPerFrame: 6, SamplesPerFrame: 1536, FrameSizeBytes: 1200},
	}
	to := ProgramSpec{
		SampleRateHz: 44100,
		Channel:      ChannelStructure{Acmod: 2, Channels: 2},
		DRC:          DRCState{ConfigID: "night-v2"},
		Boundary:     BoundaryState{BlocksPerFrame: 2, SamplesPerFrame: 512, FrameSizeBytes: 700},
	}

	events := PlanSpecChange(from, to, "adaptive bitrate switch")
	kinds := make(map[EventKind]bool)
	for _, event := range events {
		kinds[event.Kind] = true
		if event.Order == 0 {
			t.Fatal("event order must be assigned")
		}
	}
	for _, kind := range []EventKind{EventFrameResync, EventDecoder, EventMixer, EventDRC, EventOutput} {
		if !kinds[kind] {
			t.Fatalf("missing %s in %+v", kind, events)
		}
	}
}
