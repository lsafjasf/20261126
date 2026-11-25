package captionhub

import "fmt"

// Tracker is the viewer-side session guard. The zero value is not usable;
// create one with NewTracker or LoadSnapshot.
//
// Tracker is intended to be owned by the player/UI goroutine and is not safe
// for concurrent use.
type Tracker struct {
	checkpoint Checkpoint
	cues       []Cue
	cueIDs     map[string]struct{}
	rejected   int64
}

// NewTracker creates an empty tracker pinned to cp.
func NewTracker(cp Checkpoint) *Tracker {
	return &Tracker{
		checkpoint: cp,
		cues:       []Cue{},
		cueIDs:     make(map[string]struct{}),
	}
}

// LoadSnapshot creates a tracker and stores the matching cues from a snapshot.
// Cues from another session epoch are discarded.
func LoadSnapshot(cp Checkpoint, cues []Cue) *Tracker {
	tracker := NewTracker(cp)
	for _, cue := range cues {
		if cue.SessionID != cp.SessionID || cue.Epoch != cp.Epoch {
			tracker.rejected++
			continue
		}
		if _, exists := tracker.cueIDs[cue.ID]; exists {
			continue
		}
		tracker.cueIDs[cue.ID] = struct{}{}
		tracker.cues = append(tracker.cues, cue)
	}
	return tracker
}

// Reset switches the viewer to a new session epoch and clears all old cues.
func (t *Tracker) Reset(cp Checkpoint) {
	t.checkpoint = cp
	t.cues = []Cue{}
	t.cueIDs = make(map[string]struct{})
}

// Accept accepts only a cue from the tracker's current session epoch.
func (t *Tracker) Accept(cue Cue) error {
	if cue.SessionID != t.checkpoint.SessionID || cue.Epoch != t.checkpoint.Epoch {
		t.rejected++
		return fmt.Errorf("%w: viewer is on %s#e%d, cue targets %s#e%d",
			ErrStaleEpoch,
			t.checkpoint.SessionID, t.checkpoint.Epoch,
			cue.SessionID, cue.Epoch,
		)
	}
	if _, exists := t.cueIDs[cue.ID]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicateCue, cue.ID)
	}
	t.cueIDs[cue.ID] = struct{}{}
	t.cues = append(t.cues, cue)
	return nil
}

// VisibleAt returns the cues visible at pos on the current timeline.
func (t *Tracker) VisibleAt(pos Millis) []Cue {
	return SelectVisible(t.cues, t.checkpoint, pos)
}

// Checkpoint returns the current recovery token.
func (t *Tracker) Checkpoint() Checkpoint {
	return t.checkpoint
}

// Rejected returns the number of stale or otherwise rejected cues.
func (t *Tracker) Rejected() int64 {
	return t.rejected
}
