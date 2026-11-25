package timelineconstraint

import (
	"fmt"
	"math/big"
)

var (
	errInvalidTimecode = fmt.Errorf("invalid timecode")
	errInvalidRange    = fmt.Errorf("invalid range")
)

type Rate struct {
	FramesPerSecond *big.Rat
	Drop            bool
}

func NewRate(numerator, denominator int64, drop bool) Rate {
	return Rate{
		FramesPerSecond: big.NewRat(numerator, denominator),
		Drop:            drop,
	}
}

func (r Rate) clone() Rate {
	return Rate{
		FramesPerSecond: new(big.Rat).Set(r.FramesPerSecond),
		Drop:            r.Drop,
	}
}

func (r Rate) validate() error {
	if r.FramesPerSecond == nil || r.FramesPerSecond.Sign() <= 0 {
		return fmt.Errorf("rate must be positive")
	}
	nominal, err := r.nominalFPS()
	if err != nil {
		return err
	}
	if r.Drop && nominal != 30 && nominal != 60 {
		return fmt.Errorf("drop-frame timecode requires a 30 or 60 fps nominal rate")
	}
	return nil
}

func (r Rate) nominalFPS() (int64, error) {
	if err := r.validateValue(); err != nil {
		return 0, err
	}
	num := r.FramesPerSecond.Num()
	den := r.FramesPerSecond.Denom()
	quotient, remainder := new(big.Int).QuoRem(num, den, new(big.Int))
	if remainder.Sign() != 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	return quotient.Int64(), nil
}

func (r Rate) validateValue() error {
	if r.FramesPerSecond == nil || r.FramesPerSecond.Sign() <= 0 {
		return fmt.Errorf("rate must be positive")
	}
	return nil
}

type Timecode struct {
	Hour   int64
	Minute int64
	Second int64
	Frame  int64
	Drop   bool
}

type Range struct {
	In  Timecode
	Out Timecode
}

type RatedRange struct {
	Rate Rate
	In   Timecode
	Out  Timecode
}

func (tc Timecode) frameNumber(rate Rate) (int64, error) {
	if err := rate.validate(); err != nil {
		return 0, err
	}
	if tc.Drop != rate.Drop {
		return 0, fmt.Errorf("timecode drop-frame flag does not match rate")
	}
	nominal, err := rate.nominalFPS()
	if err != nil {
		return 0, err
	}
	if tc.Hour < 0 || tc.Hour >= 24 || tc.Minute < 0 || tc.Minute >= 60 ||
		tc.Second < 0 || tc.Second >= 60 || tc.Frame < 0 || tc.Frame >= nominal {
		return 0, fmt.Errorf("timecode %s is out of range", tc.String())
	}
	if tc.Drop && tc.Second == 0 && tc.Frame < nominal/15 && tc.Minute%10 != 0 {
		return 0, fmt.Errorf("timecode %s is a skipped drop-frame label", tc.String())
	}

	totalMinutes := tc.Hour*60 + tc.Minute
	frameNumber := (tc.Hour*3600+tc.Minute*60+tc.Second)*nominal + tc.Frame
	if tc.Drop {
		skipped := (nominal / 15) * (totalMinutes - totalMinutes/10)
		frameNumber -= skipped
	}
	return frameNumber, nil
}

func (tc Timecode) String() string {
	separator := byte(':')
	if tc.Drop {
		separator = ';'
	}
	return fmt.Sprintf("%02d:%02d:%02d%c%02d", tc.Hour, tc.Minute, tc.Second, separator, tc.Frame)
}

func rangeDurationFrames(in, out Timecode, rate Rate) (int64, int64, error) {
	inFrame, err := in.frameNumber(rate)
	if err != nil {
		return 0, 0, fmt.Errorf("%w: %w", errInvalidTimecode, err)
	}
	outFrame, err := out.frameNumber(rate)
	if err != nil {
		return 0, 0, fmt.Errorf("%w: %w", errInvalidTimecode, err)
	}
	if outFrame <= inFrame {
		return 0, 0, fmt.Errorf("%w: range out must be later than in", errInvalidRange)
	}
	return inFrame, outFrame - inFrame, nil
}
