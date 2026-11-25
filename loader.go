package subselect

import "context"

// Loader fetches cue payload for a track. Implementations may block on
// network/disk; Player guards out-of-order completions with a generation.
type Loader interface {
	Load(ctx context.Context, track Track) ([]Cue, error)
}

// LoadFunc adapts a function to Loader.
type LoadFunc func(ctx context.Context, track Track) ([]Cue, error)

// Load implements Loader.
func (f LoadFunc) Load(ctx context.Context, track Track) ([]Cue, error) {
	return f(ctx, track)
}

// StaticLoader returns the cues embedded in the catalog track. It is the
// default loader and never performs I/O.
type StaticLoader struct{}

// Load implements Loader.
func (StaticLoader) Load(_ context.Context, track Track) ([]Cue, error) {
	return append([]Cue(nil), track.Cues...), nil
}

// TrackLoadError records one failed candidate in a fallback chain.
type TrackLoadError struct {
	TrackID string `json:"track_id"`
	Err     error  `json:"-"`
}

// Error implements error.
func (e TrackLoadError) Error() string {
	return "subselect: load track " + e.TrackID + ": " + e.Err.Error()
}

// Unwrap exposes the underlying error.
func (e TrackLoadError) Unwrap() error { return e.Err }
