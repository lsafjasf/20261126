package aache

import (
	"errors"
	"math/big"
	"testing"
	"time"
)

func sbr(rate uint32) *SBRConfig {
	return &SBRConfig{Enabled: true, OutputRate: rate}
}

func ps(enabled bool) *PSConfig {
	return &PSConfig{Enabled: enabled}
}

func TestPersistentConfigAcrossHeaderGaps(t *testing.T) {
	t.Parallel()

	stream := Stream{
		CoreRate: 24000,
		Frames: []Frame{
			{Config: &ExtensionUpdate{SBR: sbr(48000), PS: ps(true)}},
			{},
			{Config: &ExtensionUpdate{SBR: sbr(48000)}},
			{},
			{Config: &ExtensionUpdate{SBR: &SBRConfig{}}},
			{},
		},
	}
	timeline, err := BuildTimeline(stream)
	if err != nil {
		t.Fatalf("BuildTimeline: %v", err)
	}
	if len(timeline.Frames) != 6 {
		t.Fatalf("got %d frames, want 6", len(timeline.Frames))
	}

	for index := 0; index < 4; index++ {
		frame := timeline.Frames[index]
		if frame.OutputSamples != 2048 || frame.OutputRate != 48000 {
			t.Fatalf("frame %d output: got %d/%d, want 2048/48000",
				index, frame.OutputSamples, frame.OutputRate)
		}
		if !frame.Config.SBR.Enabled || !frame.Config.PS.Enabled {
			t.Fatalf("frame %d lost SBR/PS state: %+v", index, frame.Config)
		}
	}
	for index := 4; index < 6; index++ {
		frame := timeline.Frames[index]
		if frame.OutputSamples != 1024 || frame.OutputRate != 24000 {
			t.Fatalf("frame %d after disable: got %d/%d, want 1024/24000",
				index, frame.OutputSamples, frame.OutputRate)
		}
		if frame.Config.Active() {
			t.Fatalf("frame %d still has extension state: %+v", index, frame.Config)
		}
	}

	if timeline.FirstConfigGap == nil || timeline.FirstConfigGap.Frame != 1 {
		t.Fatalf("first config gap: got %+v, want frame 1", timeline.FirstConfigGap)
	}
	if timeline.FirstOutputMismatch != nil {
		t.Fatalf("unexpected observed mismatch: %+v", timeline.FirstOutputMismatch)
	}
	if timeline.Frames[2].Config.PS.SourceFrame != 0 {
		t.Fatalf("PS source changed without a PS update: %+v", timeline.Frames[2].Config.PS)
	}
	if timeline.Frames[2].Config.SBR.SourceFrame != 2 {
		t.Fatalf("SBR source not refreshed: %+v", timeline.Frames[2].Config.SBR)
	}

	for index, frame := range timeline.Frames {
		if delta := frame.Duration() - frame.CoreDuration; delta < -time.Nanosecond || delta > time.Nanosecond {
			t.Fatalf("frame %d duration %s differs from core duration %s",
				index, frame.Duration(), frame.CoreDuration)
		}
	}
}

func TestExplicitDisableAndPSOnlyUpdate(t *testing.T) {
	t.Parallel()

	stream := Stream{
		CoreRate: 24000,
		Frames: []Frame{
			{Config: &ExtensionUpdate{SBR: sbr(48000), PS: ps(true)}},
			{Config: &ExtensionUpdate{PS: ps(false)}},
			{},
			{Config: &ExtensionUpdate{SBR: &SBRConfig{Enabled: false}}},
			{},
		},
	}
	timeline, err := BuildTimeline(stream)
	if err != nil {
		t.Fatalf("BuildTimeline: %v", err)
	}
	if !timeline.Frames[1].Config.SBR.Enabled || timeline.Frames[1].Config.PS.Enabled {
		t.Fatalf("PS-only update changed SBR or kept PS: %+v", timeline.Frames[1].Config)
	}
	if timeline.Frames[1].OutputSamples != 2048 {
		t.Fatalf("PS-only update changed output samples: %d", timeline.Frames[1].OutputSamples)
	}
	if timeline.FirstConfigGap == nil || timeline.FirstConfigGap.Frame != 2 {
		t.Fatalf("first config gap: got %+v, want frame 2", timeline.FirstConfigGap)
	}
	if timeline.Frames[4].Config.Active() {
		t.Fatalf("explicit disable did not clear extensions: %+v", timeline.Frames[4].Config)
	}
}

func TestExplicitDisableDoesNotCreateConfigGap(t *testing.T) {
	t.Parallel()

	timeline, err := BuildTimeline(Stream{
		CoreRate: 24000,
		Frames: []Frame{
			{Config: &ExtensionUpdate{SBR: sbr(48000)}},
			{Config: &ExtensionUpdate{SBR: &SBRConfig{Enabled: false}}},
			{},
		},
	})
	if err != nil {
		t.Fatalf("BuildTimeline: %v", err)
	}
	if timeline.FirstConfigGap != nil {
		t.Fatalf("explicit disable reported as gap: %+v", timeline.FirstConfigGap)
	}
}

func TestPSRequiresSBR(t *testing.T) {
	t.Parallel()

	_, err := BuildTimeline(Stream{
		CoreRate: 24000,
		Frames: []Frame{
			{Config: &ExtensionUpdate{PS: ps(true)}},
		},
	})
	if !errors.Is(err, ErrPSWithoutSBR) {
		t.Fatalf("got %v, want ErrPSWithoutSBR", err)
	}
}

func TestObservedOutputMismatchIsReported(t *testing.T) {
	t.Parallel()

	timeline, err := BuildTimeline(Stream{
		CoreRate: 24000,
		Frames: []Frame{
			{Config: &ExtensionUpdate{SBR: sbr(48000)}},
			{OutputSamples: 1024, OutputRate: 48000},
		},
	})
	if err != nil {
		t.Fatalf("BuildTimeline: %v", err)
	}
	if timeline.Frames[1].OutputSamples != 1024 || timeline.Frames[1].OutputRate != 48000 {
		t.Fatalf("observed count was not authoritative: %+v", timeline.Frames[1])
	}
	mismatch := timeline.FirstOutputMismatch
	if mismatch == nil || mismatch.Frame != 1 {
		t.Fatalf("first mismatch: got %+v, want frame 1", mismatch)
	}
	if mismatch.ExpectedSamples != 2048 || mismatch.ObservedSamples != 1024 {
		t.Fatalf("mismatch counts: %+v", mismatch)
	}
}

func TestMultipleStreamsInheritExtensionState(t *testing.T) {
	t.Parallel()

	streams := []Stream{
		{
			Name:     "source-a",
			CoreRate: 24000,
			Frames: []Frame{
				{Config: &ExtensionUpdate{SBR: sbr(48000), PS: ps(true)}},
				{},
			},
		},
		{
			Name:     "source-b",
			CoreRate: 24000,
			Inherit:  true,
			Frames:   []Frame{{}, {}},
		},
	}
	timeline, err := BuildStreams(streams...)
	if err != nil {
		t.Fatalf("BuildStreams: %v", err)
	}
	for index, frame := range timeline.Frames {
		if frame.OutputSamples != 2048 || frame.OutputRate != 48000 {
			t.Fatalf("frame %d lost inherited SBR: %+v", index, frame)
		}
	}
	if len(timeline.Streams) != 2 || timeline.Streams[1].StartFrame != 2 {
		t.Fatalf("stream boundaries: %+v", timeline.Streams)
	}
	if timeline.FirstConfigGap == nil || timeline.FirstConfigGap.Frame != 1 {
		t.Fatalf("first gap: %+v", timeline.FirstConfigGap)
	}
}

func TestStreamInitialConfigAppliesAtBoundary(t *testing.T) {
	t.Parallel()

	timeline, err := BuildStreams(
		Stream{Name: "a", CoreRate: 24000, Frames: []Frame{{}}},
		Stream{
			Name:     "b",
			CoreRate: 24000,
			Initial:  &ExtensionUpdate{SBR: sbr(48000)},
			Frames:   []Frame{{}, {}},
		},
	)
	if err != nil {
		t.Fatalf("BuildStreams: %v", err)
	}
	if timeline.Frames[0].OutputSamples != 1024 {
		t.Fatalf("first stream should be core-only: %+v", timeline.Frames[0])
	}
	for index := 1; index < 3; index++ {
		if timeline.Frames[index].OutputSamples != 2048 {
			t.Fatalf("frame %d did not apply stream initial config: %+v", index, timeline.Frames[index])
		}
	}
}

func TestFractionalLongRunDoesNotDrift(t *testing.T) {
	t.Parallel()

	const frameCount = 1001
	frames := make([]Frame, frameCount)
	frames[0].Config = &ExtensionUpdate{SBR: sbr(44100)}
	timeline, err := BuildTimeline(Stream{CoreRate: 22050, Frames: frames})
	if err != nil {
		t.Fatalf("BuildTimeline: %v", err)
	}
	if timeline.OutputSamples != frameCount*2048 {
		t.Fatalf("output samples: got %d, want %d", timeline.OutputSamples, frameCount*2048)
	}
	wantDuration := ratDuration(new(big.Rat).SetFrac64(frameCount*2048, 44100))
	if timeline.Duration != wantDuration {
		t.Fatalf("duration: got %s, want %s", timeline.Duration, wantDuration)
	}
	for index, frame := range timeline.Frames {
		if frame.Duration() != frame.CoreDuration {
			t.Fatalf("frame %d duration %s differs from core %s",
				index, frame.Duration(), frame.CoreDuration)
		}
	}
}

func TestInvalidInputIsRejected(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		stream Stream
		want   error
	}{
		{
			name:   "missing core rate",
			stream: Stream{Frames: []Frame{{}}},
			want:   ErrCoreRate,
		},
		{
			name: "negative core samples",
			stream: Stream{
				CoreRate: 24000,
				Frames:   []Frame{{CoreSamples: -1}},
			},
			want: ErrCoreSamples,
		},
		{
			name: "sbr rate below core",
			stream: Stream{
				CoreRate: 48000,
				Frames: []Frame{
					{Config: &ExtensionUpdate{SBR: sbr(24000)}},
				},
			},
			want: ErrSBRRate,
		},
		{
			name: "negative observed samples",
			stream: Stream{
				CoreRate: 24000,
				Frames:   []Frame{{OutputSamples: -1}},
			},
			want: ErrObservedSamples,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := BuildTimeline(test.stream)
			if !errors.Is(err, test.want) {
				t.Fatalf("got %v, want %v", err, test.want)
			}
		})
	}

	if _, err := BuildWithOptions(
		[]Stream{{CoreRate: 48000}},
		Options{OutputRate: 24000},
	); !errors.Is(err, ErrOutputRate) {
		t.Fatalf("fixed rate below core: got %v, want ErrOutputRate", err)
	}
	if _, err := BuildWithOptions(
		[]Stream{{CoreRate: 24000}},
		Options{Policy: ExtensionPolicy("nonsense")},
	); err == nil {
		t.Fatal("unknown policy was accepted")
	}
}
