package safereflow

import "errors"

// Sentinel errors used by loading, planning and application.
var (
	ErrNilDocument   = errors.New("safereflow: nil cue document")
	ErrNilTrack      = errors.New("safereflow: nil safe track")
	ErrPlanNotFound  = errors.New("safereflow: plan not found")
	ErrPlanBlocked   = errors.New("safereflow: plan not applicable")
	ErrLowConfidence = errors.New("safereflow: migration blocked by low safe-area confidence")
	ErrNoCue         = errors.New("safereflow: cue not found")
)
