package aache

import "time"

// Divergence is one frame where the persistent and frame-local mappings
// disagree. The first divergence is the first place per-frame handling loses
// configuration.
type Divergence struct {
	Frame              int            `json:"frame"`
	StreamIndex        int            `json:"streamIndex"`
	FrameInStream      int            `json:"frameInStream"`
	PersistedConfig    ExtensionState `json:"persistedConfig"`
	FrameLocalConfig   ExtensionState `json:"frameLocalConfig"`
	PersistedSamples   int64          `json:"persistedSamples"`
	FrameLocalSamples  int64          `json:"frameLocalSamples"`
	PersistedRate      uint32         `json:"persistedRate"`
	FrameLocalRate     uint32         `json:"frameLocalRate"`
	PersistedStart     time.Duration  `json:"persistedStart"`
	FrameLocalStart    time.Duration  `json:"frameLocalStart"`
	PersistedEnd       time.Duration  `json:"persistedEnd"`
	FrameLocalEnd      time.Duration  `json:"frameLocalEnd"`
	PersistedDuration  time.Duration  `json:"persistedDuration"`
	FrameLocalDuration time.Duration  `json:"frameLocalDuration"`
}

// Comparison contains both mappings. Persisted is the repaired timeline;
// FrameLocal reproduces the editor that forgets configuration on frames without
// an SBR/PS header.
type Comparison struct {
	Persisted           *Timeline     `json:"persisted"`
	FrameLocal          *Timeline     `json:"frameLocal"`
	ReferenceOutputRate uint32        `json:"referenceOutputRate"`
	FirstLoss           *Divergence   `json:"firstLoss,omitempty"`
	DivergenceCount     int           `json:"divergenceCount"`
	DurationDelta       time.Duration `json:"durationDelta"`
}

// CompareStream is the single-stream convenience form of Compare.
func CompareStream(stream Stream) (*Comparison, error) {
	return Compare([]Stream{stream})
}

// Compare builds the persistent and frame-local mappings from the same input
// and reports the first frame where they diverge.
func Compare(streams []Stream) (*Comparison, error) {
	persisted, err := BuildWithOptions(streams, Options{Policy: PersistExtensions})
	if err != nil {
		return nil, err
	}
	referenceRate := referenceOutputRate(streams, persisted)
	frameLocal, err := BuildWithOptions(streams, Options{
		Policy:     FrameLocalExtensions,
		OutputRate: referenceRate,
	})
	if err != nil {
		return nil, err
	}

	comparison := &Comparison{
		Persisted:           persisted,
		FrameLocal:          frameLocal,
		ReferenceOutputRate: referenceRate,
		DurationDelta:       persisted.Duration - frameLocal.Duration,
	}
	for index := range persisted.Frames {
		good := persisted.Frames[index]
		broken := frameLocal.Frames[index]
		if sameConfig(good.Config, broken.Config) &&
			good.OutputSamples == broken.OutputSamples &&
			good.OutputRate == broken.OutputRate &&
			good.Start == broken.Start {
			continue
		}
		comparison.DivergenceCount++
		if comparison.FirstLoss == nil {
			comparison.FirstLoss = &Divergence{
				Frame:              good.Index,
				StreamIndex:        good.StreamIndex,
				FrameInStream:      good.FrameInStream,
				PersistedConfig:    good.Config,
				FrameLocalConfig:   broken.Config,
				PersistedSamples:   good.OutputSamples,
				FrameLocalSamples:  broken.OutputSamples,
				PersistedRate:      good.OutputRate,
				FrameLocalRate:     broken.OutputRate,
				PersistedStart:     good.Start,
				FrameLocalStart:    broken.Start,
				PersistedEnd:       good.End,
				FrameLocalEnd:      broken.End,
				PersistedDuration:  good.Duration(),
				FrameLocalDuration: broken.Duration(),
			}
		}
	}
	return comparison, nil
}

func referenceOutputRate(streams []Stream, persisted *Timeline) uint32 {
	var reference uint32
	for _, stream := range streams {
		if stream.DeclaredOutputRate > reference {
			reference = stream.DeclaredOutputRate
		}
		if stream.CoreRate > reference {
			reference = stream.CoreRate
		}
	}
	for _, frame := range persisted.Frames {
		if frame.OutputRate > reference {
			reference = frame.OutputRate
		}
		if frame.ExpectedOutputRate > reference {
			reference = frame.ExpectedOutputRate
		}
	}
	return reference
}
