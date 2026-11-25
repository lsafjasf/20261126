package aache

import "testing"

func TestCompareFindsFirstFrameLocalConfigLoss(t *testing.T) {
	t.Parallel()

	comparison, err := CompareStream(Stream{
		CoreRate: 24000,
		Frames: []Frame{
			{Config: &ExtensionUpdate{SBR: sbr(48000), PS: ps(true)}},
			{},
			{},
		},
	})
	if err != nil {
		t.Fatalf("CompareStream: %v", err)
	}
	if comparison.ReferenceOutputRate != 48000 {
		t.Fatalf("reference rate: got %d, want 48000", comparison.ReferenceOutputRate)
	}
	loss := comparison.FirstLoss
	if loss == nil {
		t.Fatal("no divergence found")
	}
	if loss.Frame != 1 {
		t.Fatalf("first loss: got frame %d, want 1", loss.Frame)
	}
	if !loss.PersistedConfig.SBR.Enabled || loss.FrameLocalConfig.Active() {
		t.Fatalf("state at first loss: persisted=%+v local=%+v",
			loss.PersistedConfig, loss.FrameLocalConfig)
	}
	if loss.PersistedSamples != 2048 || loss.FrameLocalSamples != 1024 {
		t.Fatalf("samples at first loss: persisted=%d local=%d",
			loss.PersistedSamples, loss.FrameLocalSamples)
	}
	if loss.PersistedRate != 48000 || loss.FrameLocalRate != 48000 {
		t.Fatalf("rates at first loss: persisted=%d local=%d",
			loss.PersistedRate, loss.FrameLocalRate)
	}
	if loss.FrameLocalDuration >= loss.PersistedDuration {
		t.Fatalf("frame-local duration %s should be shorter than %s",
			loss.FrameLocalDuration, loss.PersistedDuration)
	}
	if comparison.Persisted.Duration <= comparison.FrameLocal.Duration {
		t.Fatalf("persisted duration %s should exceed frame-local %s",
			comparison.Persisted.Duration, comparison.FrameLocal.Duration)
	}
}

func TestCompareKeepsConfigurationAcrossStreamsWhenInherited(t *testing.T) {
	t.Parallel()

	comparison, err := Compare([]Stream{
		{
			Name:     "a",
			CoreRate: 24000,
			Frames: []Frame{
				{Config: &ExtensionUpdate{SBR: sbr(48000)}},
			},
		},
		{
			Name:     "b",
			CoreRate: 24000,
			Inherit:  true,
			Frames:   []Frame{{}, {}},
		},
	})
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if comparison.FirstLoss == nil || comparison.FirstLoss.Frame != 1 {
		t.Fatalf("first loss: %+v", comparison.FirstLoss)
	}
	for _, frame := range comparison.Persisted.Frames {
		if frame.OutputSamples != 2048 {
			t.Fatalf("persisted frame %d lost SBR: %+v", frame.Index, frame)
		}
	}
}

func TestCompareWithoutConfigHasNoDivergence(t *testing.T) {
	t.Parallel()

	comparison, err := CompareStream(Stream{
		CoreRate: 24000,
		Frames:   []Frame{{}, {}, {}},
	})
	if err != nil {
		t.Fatalf("CompareStream: %v", err)
	}
	if comparison.FirstLoss != nil {
		t.Fatalf("unexpected loss: %+v", comparison.FirstLoss)
	}
	if comparison.DivergenceCount != 0 {
		t.Fatalf("got %d divergences, want 0", comparison.DivergenceCount)
	}
}
