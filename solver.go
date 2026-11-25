package timelineconstraint

import (
	"errors"
	"fmt"
	"math/big"
)

const (
	ConflictRateMismatch       = "recording_rate_mismatch"
	ConflictInvalidTimecode    = "invalid_timecode"
	ConflictInvalidRange       = "invalid_range"
	ConflictInvalidSpeed       = "invalid_speed"
	ConflictInvalidRate        = "invalid_rate"
	ConflictRecordingRateCheck = "recording_rate_check_failed"
)

type Constraint struct {
	TimecodeRate Rate
	Source       Range
	Record       RatedRange
	Speed        *big.Rat
}

type Result struct {
	SourceInFrame       int64
	SourceDurationFrame int64
	RecordInFrame       int64
	RecordDurationFrame int64

	TimecodeRate Rate
	RecordRate   Rate
	ActualRate   *big.Rat
	Speed        *big.Rat

	SourceTimecodeDuration *big.Rat
	SourceMediaDuration    *big.Rat
	RecordDuration         *big.Rat

	SourceStartTimecodeTime *big.Rat
	SourceStartMediaTime    *big.Rat
	AudioStartOffset        *big.Rat
	AudioDurationDrift      *big.Rat

	Conflicts []Conflict
}

type Conflict struct {
	Code     string
	Message  string
	Expected *big.Rat
	Actual   *big.Rat
}

type ConflictError struct {
	Conflicts []Conflict
}

func (e ConflictError) Error() string {
	if len(e.Conflicts) == 0 {
		return "constraint conflict"
	}
	return e.Conflicts[0].Message
}

func Solve(constraint Constraint) (Result, error) {
	return solve(constraint, false)
}

func SolveStrict(constraint Constraint) (Result, error) {
	return solve(constraint, true)
}

func solve(constraint Constraint, strict bool) (Result, error) {
	if constraint.Speed == nil || constraint.Speed.Sign() <= 0 {
		return Result{}, ConflictError{Conflicts: []Conflict{{
			Code:    ConflictInvalidSpeed,
			Message: "speed must be positive",
		}}}
	}
	if err := constraint.TimecodeRate.validate(); err != nil {
		return Result{}, ConflictError{Conflicts: []Conflict{{
			Code:    ConflictInvalidRate,
			Message: err.Error(),
		}}}
	}
	if err := constraint.Record.Rate.validate(); err != nil {
		return Result{}, ConflictError{Conflicts: []Conflict{{
			Code:    ConflictInvalidRate,
			Message: err.Error(),
		}}}
	}

	sourceIn, sourceFrames, err := rangeDurationFrames(
		constraint.Source.In,
		constraint.Source.Out,
		constraint.TimecodeRate,
	)
	if err != nil {
		return Result{}, ConflictError{Conflicts: []Conflict{classifyRangeError(err)}}
	}
	recordIn, recordFrames, err := rangeDurationFrames(
		constraint.Record.In,
		constraint.Record.Out,
		constraint.Record.Rate,
	)
	if err != nil {
		return Result{}, ConflictError{Conflicts: []Conflict{classifyRangeError(err)}}
	}

	recordRate := constraint.Record.Rate.FramesPerSecond
	timecodeRate := constraint.TimecodeRate.FramesPerSecond
	speed := new(big.Rat).Set(constraint.Speed)

	recordDuration := new(big.Rat).Quo(
		big.NewRat(recordFrames, 1),
		new(big.Rat).Set(recordRate),
	)
	sourceTimecodeDuration := new(big.Rat).Quo(
		big.NewRat(sourceFrames, 1),
		new(big.Rat).Set(timecodeRate),
	)

	actualRate := new(big.Rat).Quo(
		big.NewRat(sourceFrames, 1),
		new(big.Rat).Mul(speed, recordDuration),
	)
	sourceMediaDuration := new(big.Rat).Quo(
		big.NewRat(sourceFrames, 1),
		new(big.Rat).Set(actualRate),
	)

	sourceStartTimecodeTime := new(big.Rat).Quo(
		big.NewRat(sourceIn, 1),
		new(big.Rat).Set(timecodeRate),
	)
	sourceStartMediaTime := new(big.Rat).Quo(
		big.NewRat(sourceIn, 1),
		new(big.Rat).Set(actualRate),
	)
	audioStartOffset := new(big.Rat).Sub(sourceStartMediaTime, sourceStartTimecodeTime)
	audioDurationDrift := new(big.Rat).Sub(sourceMediaDuration, sourceTimecodeDuration)

	result := Result{
		SourceInFrame:           sourceIn,
		SourceDurationFrame:     sourceFrames,
		RecordInFrame:           recordIn,
		RecordDurationFrame:     recordFrames,
		TimecodeRate:            constraint.TimecodeRate.clone(),
		RecordRate:              constraint.Record.Rate.clone(),
		ActualRate:              actualRate,
		Speed:                   speed,
		SourceTimecodeDuration:  sourceTimecodeDuration,
		SourceMediaDuration:     sourceMediaDuration,
		RecordDuration:          recordDuration,
		SourceStartTimecodeTime: sourceStartTimecodeTime,
		SourceStartMediaTime:    sourceStartMediaTime,
		AudioStartOffset:        audioStartOffset,
		AudioDurationDrift:      audioDurationDrift,
	}

	if actualRate.Cmp(timecodeRate) != 0 {
		result.Conflicts = append(result.Conflicts, Conflict{
			Code: ConflictRateMismatch,
			Message: fmt.Sprintf(
				"source timecode rate %s fps cannot also be the recording rate: source range, record range, and speed imply %s fps",
				timecodeRate.RatString(),
				actualRate.RatString(),
			),
			Expected: new(big.Rat).Set(timecodeRate),
			Actual:   new(big.Rat).Set(actualRate),
		})
		if strict {
			return Result{}, ConflictError{Conflicts: result.Conflicts}
		}
	}

	return result, nil
}

func CheckRecordingRate(result Result, measured Rate) error {
	if err := measured.validate(); err != nil {
		return ConflictError{Conflicts: []Conflict{{
			Code:    ConflictInvalidRate,
			Message: err.Error(),
		}}}
	}
	if result.ActualRate == nil {
		return ConflictError{Conflicts: []Conflict{{
			Code:    ConflictRecordingRateCheck,
			Message: "result does not contain an actual rate",
		}}}
	}
	if result.ActualRate.Cmp(measured.FramesPerSecond) != 0 {
		return ConflictError{Conflicts: []Conflict{{
			Code:     ConflictRecordingRateCheck,
			Message:  fmt.Sprintf("measured recording rate %s fps does not match implied rate %s fps", measured.FramesPerSecond.RatString(), result.ActualRate.RatString()),
			Expected: new(big.Rat).Set(result.ActualRate),
			Actual:   new(big.Rat).Set(measured.FramesPerSecond),
		}}}
	}
	return nil
}

func (r Result) HasConflict(code string) bool {
	for _, conflict := range r.Conflicts {
		if conflict.Code == code {
			return true
		}
	}
	return false
}

func classifyRangeError(err error) Conflict {
	if errors.Is(err, errInvalidTimecode) {
		return Conflict{
			Code:    ConflictInvalidTimecode,
			Message: err.Error(),
		}
	}
	return Conflict{
		Code:    ConflictInvalidRange,
		Message: err.Error(),
	}
}

func RatFloatString(value *big.Rat) string {
	if value == nil {
		return ""
	}
	float, _ := value.Float64()
	return fmt.Sprintf("%.12g", float)
}
