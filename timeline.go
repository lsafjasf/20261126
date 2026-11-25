package aache

import (
	"fmt"
	"math/big"
	"time"
)

// Options controls timeline construction.
type Options struct {
	// Policy defaults to PersistExtensions when empty.
	Policy ExtensionPolicy
	// OutputRate, when non-zero, forces the presentation sample rate for every
	// frame. Output sample counts still come from the effective extension state
	// unless a frame supplies an observed count.
	OutputRate uint32
}

// BuildTimeline maps one stream with the persistent extension rule.
func BuildTimeline(stream Stream) (*Timeline, error) {
	return BuildWithOptions([]Stream{stream}, Options{})
}

// BuildStreams maps one or more streams. Each stream starts from its own
// Initial configuration unless Inherit is set on the stream.
func BuildStreams(streams ...Stream) (*Timeline, error) {
	return BuildWithOptions(streams, Options{})
}

// BuildWithOptions maps streams using an explicit policy and optional fixed
// output rate.
func BuildWithOptions(streams []Stream, opts Options) (*Timeline, error) {
	policy := opts.Policy
	if policy == "" {
		policy = PersistExtensions
	}
	if policy != PersistExtensions && policy != FrameLocalExtensions {
		return nil, fmt.Errorf("aache: unknown extension policy %q", policy)
	}
	if opts.OutputRate != 0 && policy == PersistExtensions {
		// A fixed rate is legal, but a rate below the core rate can only be a
		// caller error.
		for _, stream := range streams {
			if stream.CoreRate != 0 && opts.OutputRate < stream.CoreRate {
				return nil, fmt.Errorf("%w: fixed %d, core %d",
					ErrOutputRate, opts.OutputRate, stream.CoreRate)
			}
		}
	}

	timeline := &Timeline{Policy: policy}
	clock := new(big.Rat)
	var state ExtensionState
	var coreTotal, outputTotal int64
	globalIndex := 0
	lastInBandConfig := -1
	sawInBandConfig := false

	for streamIndex, stream := range streams {
		if stream.CoreRate == 0 {
			return nil, fmt.Errorf("%w: stream %d", ErrCoreRate, streamIndex)
		}
		streamCoreSamples := defaultCoreSamples(stream.CoreSamples)
		if streamCoreSamples <= 0 {
			return nil, fmt.Errorf("%w: stream %d", ErrCoreSamples, streamIndex)
		}

		if policy == FrameLocalExtensions || !stream.Inherit || streamIndex == 0 {
			state = ExtensionState{}
		}
		if policy == PersistExtensions && stream.Initial != nil {
			if err := state.applyUpdate(stream.Initial, stream.CoreRate, globalIndex, -1); err != nil {
				return nil, fmt.Errorf("aache: stream %d initial config: %w", streamIndex, err)
			}
			sawInBandConfig = false
			lastInBandConfig = -1
		}

		streamStartFrame := globalIndex
		streamStartOutput := outputTotal
		streamStartTime := ratDuration(clock)

		for frameIndex, frame := range stream.Frames {
			coreRate := frame.CoreRate
			if coreRate == 0 {
				coreRate = stream.CoreRate
			}
			if coreRate == 0 {
				return nil, fmt.Errorf("%w: frame %d", ErrCoreRate, globalIndex)
			}
			coreSamples := frame.CoreSamples
			if coreSamples == 0 {
				coreSamples = streamCoreSamples
			}
			if coreSamples < 0 {
				return nil, fmt.Errorf("%w: frame %d", ErrCoreSamples, globalIndex)
			}
			if frame.OutputSamples < 0 {
				return nil, fmt.Errorf("%w: frame %d", ErrObservedSamples, globalIndex)
			}

			previous := state
			if policy == FrameLocalExtensions {
				state = ExtensionState{}
				if frameIndex == 0 && stream.Initial != nil {
					if err := applyFrameLocal(&state, stream.Initial, coreRate, globalIndex); err != nil {
						return nil, fmt.Errorf("aache: stream %d initial config: %w", streamIndex, err)
					}
				}
				if err := applyFrameLocal(&state, frame.Config, coreRate, globalIndex); err != nil {
					return nil, fmt.Errorf("aache: frame %d config: %w", globalIndex, err)
				}
			} else {
				if err := state.applyUpdate(frame.Config, coreRate, globalIndex, globalIndex); err != nil {
					return nil, fmt.Errorf("aache: frame %d config: %w", globalIndex, err)
				}
				if hasUpdate(frame.Config) && state.Active() {
					sawInBandConfig = true
					lastInBandConfig = globalIndex
				}
			}

			expectedSamples, expectedRate, err := state.expectedOutput(coreSamples, coreRate)
			if err != nil {
				return nil, fmt.Errorf("aache: frame %d: %w", globalIndex, err)
			}
			actualSamples := expectedSamples
			observed := frame.OutputSamples > 0
			if observed {
				actualSamples = frame.OutputSamples
			}
			actualRate := chooseOutputRate(frame, opts.OutputRate, expectedSamples, expectedRate, actualSamples, coreSamples, coreRate)
			if actualRate == 0 {
				return nil, fmt.Errorf("%w: frame %d", ErrOutputRate, globalIndex)
			}
			mismatch := observed && (actualSamples != expectedSamples || actualRate != expectedRate)

			frameStart := new(big.Rat).Set(clock)
			frameDuration := new(big.Rat).SetFrac64(actualSamples, int64(actualRate))
			presentationDuration := ratDuration(frameDuration)
			clock.Add(clock, frameDuration)

			coreDuration := ratDuration(new(big.Rat).SetFrac64(coreSamples, int64(coreRate)))
			timing := FrameTiming{
				Index:                 globalIndex,
				StreamIndex:           streamIndex,
				FrameInStream:         frameIndex,
				CoreStart:             coreTotal,
				CoreSamples:           coreSamples,
				CoreRate:              coreRate,
				CoreDuration:          coreDuration,
				Config:                state,
				ConfigChanged:         !sameConfig(previous, state),
				ConfigSource:          configSource(state),
				ConfigEffectiveAt:     configEffectiveAt(state),
				ExpectedOutputSamples: expectedSamples,
				ExpectedOutputRate:    expectedRate,
				OutputSamples:         actualSamples,
				OutputRate:            actualRate,
				OutputStart:           outputTotal,
				ObservedSamples:       observed,
				OutputMismatch:        mismatch,
				PresentationDuration:  presentationDuration,
				Start:                 ratDuration(frameStart),
				End:                   ratDuration(clock),
			}
			if mismatch && timeline.FirstOutputMismatch == nil {
				timeline.FirstOutputMismatch = &OutputMismatch{
					Frame:           globalIndex,
					StreamIndex:     streamIndex,
					FrameInStream:   frameIndex,
					Effective:       state,
					ExpectedSamples: expectedSamples,
					ObservedSamples: actualSamples,
					ExpectedRate:    expectedRate,
					ObservedRate:    actualRate,
					Reason:          mismatchReason(actualSamples, expectedSamples, actualRate, expectedRate, coreSamples),
				}
			}
			if policy == PersistExtensions && timeline.FirstConfigGap == nil &&
				frame.Config == nil && sawInBandConfig && state.Active() {
				timeline.FirstConfigGap = &ConfigGap{
					Frame:         globalIndex,
					StreamIndex:   streamIndex,
					FrameInStream: frameIndex,
					LastConfig:    lastInBandConfig,
					Effective:     state,
					Reason:        "frame has no SBR/PS header but the last in-band header remains effective",
				}
			}

			timeline.Frames = append(timeline.Frames, timing)
			coreTotal += coreSamples
			outputTotal += actualSamples
			globalIndex++
		}

		timeline.Streams = append(timeline.Streams, StreamTiming{
			Name:            stream.Name,
			StartFrame:      streamStartFrame,
			EndFrame:        globalIndex - 1,
			Start:           streamStartTime,
			End:             ratDuration(clock),
			OutputStart:     streamStartOutput,
			OutputSamples:   outputTotal - streamStartOutput,
			DeclaredOutRate: stream.DeclaredOutputRate,
		})
	}

	timeline.CoreSamples = coreTotal
	timeline.OutputSamples = outputTotal
	timeline.Duration = ratDuration(clock)
	return timeline, nil
}

func applyFrameLocal(state *ExtensionState, update *ExtensionUpdate, coreRate uint32, frame int) error {
	if update == nil {
		return nil
	}
	if update.SBR != nil {
		if err := state.applyUpdate(&ExtensionUpdate{SBR: update.SBR}, coreRate, frame, frame); err != nil {
			return err
		}
	}
	if update.PS != nil && state.SBR.Enabled {
		if err := state.applyUpdate(&ExtensionUpdate{PS: update.PS}, coreRate, frame, frame); err != nil {
			return err
		}
	}
	return nil
}

func chooseOutputRate(frame Frame, fixed uint32, expectedSamples int64, expectedRate uint32, actualSamples, coreSamples int64, coreRate uint32) uint32 {
	if frame.OutputRate != 0 {
		return frame.OutputRate
	}
	if fixed != 0 {
		return fixed
	}
	if actualSamples > 0 && actualSamples == coreSamples && expectedSamples > coreSamples {
		return coreRate
	}
	return expectedRate
}

func configSource(state ExtensionState) int {
	switch {
	case state.SBR.Enabled:
		return state.SBR.SourceFrame
	case state.PS.Enabled:
		return state.PS.SourceFrame
	default:
		return -1
	}
}

func configEffectiveAt(state ExtensionState) int {
	switch {
	case state.SBR.Enabled:
		return state.SBR.EffectiveFrame
	case state.PS.Enabled:
		return state.PS.EffectiveFrame
	default:
		return -1
	}
}

func mismatchReason(actualSamples, expectedSamples int64, actualRate, expectedRate uint32, coreSamples int64) string {
	switch {
	case actualSamples == coreSamples && expectedSamples > coreSamples:
		return "observed core-only sample count while SBR is effective"
	case actualRate != expectedRate:
		return "observed output rate differs from effective SBR state"
	case actualSamples != expectedSamples:
		return "observed output sample count differs from effective extension state"
	default:
		return ""
	}
}

func ratDuration(value *big.Rat) time.Duration {
	scaled := new(big.Rat).Mul(value, big.NewRat(int64(time.Second), 1))
	numerator := new(big.Int).Set(scaled.Num())
	denominator := new(big.Int).Set(scaled.Denom())
	negative := numerator.Sign() < 0
	if negative {
		numerator.Neg(numerator)
	}
	quotient, remainder := new(big.Int), new(big.Int)
	quotient.QuoRem(numerator, denominator, remainder)
	if new(big.Int).Lsh(remainder, 1).Cmp(denominator) >= 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	if negative {
		quotient.Neg(quotient)
	}
	const (
		maxInt64 = int64(^uint64(0) >> 1)
		minInt64 = -maxInt64 - 1
	)
	if !quotient.IsInt64() {
		if negative {
			return time.Duration(minInt64)
		}
		return time.Duration(maxInt64)
	}
	return time.Duration(quotient.Int64())
}
