package aache

import (
	"fmt"
	"math"
)

func defaultCoreSamples(value int64) int64 {
	if value == 0 {
		return DefaultCoreSamplesPerFrame
	}
	return value
}

func defaultSBRRate(coreRate uint32) (uint32, error) {
	if coreRate == 0 {
		return 0, ErrCoreRate
	}
	if uint64(coreRate)*DefaultSBRFactor > math.MaxUint32 {
		return 0, fmt.Errorf("%w: default SBR rate overflows uint32", ErrSBRRate)
	}
	return coreRate * DefaultSBRFactor, nil
}

// applyUpdate applies only the headers present in update. A nil member is not
// a reset. Disabling SBR also disables PS because PS has no meaning without an
// SBR layer.
func (s *ExtensionState) applyUpdate(update *ExtensionUpdate, coreRate uint32, frame, source int) error {
	if update == nil {
		return nil
	}
	if update.SBR != nil {
		if update.SBR.Enabled {
			rate := update.SBR.OutputRate
			if rate == 0 {
				var err error
				rate, err = defaultSBRRate(coreRate)
				if err != nil {
					return err
				}
			}
			if rate <= coreRate {
				return fmt.Errorf("%w: SBR %d, core %d", ErrSBRRate, rate, coreRate)
			}
			s.SBR = SBRState{
				Enabled:        true,
				OutputRate:     rate,
				SourceFrame:    source,
				EffectiveFrame: frame,
			}
		} else {
			s.SBR = SBRState{SourceFrame: source, EffectiveFrame: frame}
			s.PS = PSState{SourceFrame: source, EffectiveFrame: frame}
		}
	}
	if update.PS != nil {
		if update.PS.Enabled && !s.SBR.Enabled {
			return fmt.Errorf("%w: frame %d", ErrPSWithoutSBR, frame)
		}
		s.PS = PSState{
			Enabled:        update.PS.Enabled,
			SourceFrame:    source,
			EffectiveFrame: frame,
		}
	}
	return nil
}

func (s ExtensionState) outputRate(coreRate uint32) (uint32, error) {
	if coreRate == 0 {
		return 0, ErrCoreRate
	}
	if s.SBR.Enabled {
		if s.SBR.OutputRate <= coreRate {
			return 0, fmt.Errorf("%w: SBR %d, core %d", ErrSBRRate, s.SBR.OutputRate, coreRate)
		}
		return s.SBR.OutputRate, nil
	}
	return coreRate, nil
}

func (s ExtensionState) expectedOutput(coreSamples int64, coreRate uint32) (int64, uint32, error) {
	if coreSamples <= 0 {
		return 0, 0, ErrCoreSamples
	}
	outputRate, err := s.outputRate(coreRate)
	if err != nil {
		return 0, 0, err
	}
	if coreSamples > math.MaxInt64/int64(outputRate) {
		return 0, 0, fmt.Errorf("%w: sample count overflows int64", ErrUnevenRatio)
	}
	scaled := coreSamples * int64(outputRate)
	if scaled%int64(coreRate) != 0 {
		return 0, 0, fmt.Errorf("%w: %d core samples at %d/%d Hz",
			ErrUnevenRatio, coreSamples, outputRate, coreRate)
	}
	return scaled / int64(coreRate), outputRate, nil
}

func sameConfig(a, b ExtensionState) bool {
	return a.SBR.Enabled == b.SBR.Enabled &&
		a.SBR.OutputRate == b.SBR.OutputRate &&
		a.PS.Enabled == b.PS.Enabled
}

func hasUpdate(update *ExtensionUpdate) bool {
	return update != nil && (update.SBR != nil || update.PS != nil)
}
