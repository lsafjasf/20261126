package clipfade

import "errors"

// Stable sentinel errors so callers can branch with errors.Is.
var (
	// ErrStaleReference is returned when a FadeReference is resolved after
	// the cut geometry changed. The reference must be discarded and a fresh
	// one obtained from Model.Reference.
	ErrStaleReference = errors.New("clipfade: stale fade reference")

	ErrClipNotFound       = errors.New("clipfade: clip not found")
	ErrFadeNotFound       = errors.New("clipfade: crossfade not found")
	ErrClipExists         = errors.New("clipfade: clip already exists")
	ErrInvalidSampleRate  = errors.New("clipfade: sample rate must be positive")
	ErrInvalidDuration    = errors.New("clipfade: fade duration must be at least two samples")
	ErrClipsNotAdjacent   = errors.New("clipfade: clips are not adjacent at a shared cut point")
	ErrInsufficientHandle = errors.New("clipfade: media handle is too short for crossfade geometry")
	ErrEmptyAnalysis      = errors.New("clipfade: not enough samples for phase analysis")
	ErrNoZeroCrossing     = errors.New("clipfade: no zero crossing within search window")
	ErrNothingToRebuild   = errors.New("clipfade: fade has no derivable curve")
)
