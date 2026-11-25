package handlefix

import (
	"errors"
	"fmt"
)

// StepHandleTrim is the export/conform step that historically trimmed source
// edges inward instead of extending them into reserved media handles.
const StepHandleTrim = "trim_handles_for_export"

// Clip is one sequence item. Source and Sequence use different coordinate
// origins, but this minimal model assumes unit rate, so their durations must
// agree before transition overscan is added to both spaces.
type Clip struct {
	ID       string `json:"id"`
	Media    Range  `json:"media"`
	Source   Range  `json:"source_range"`
	Sequence Range  `json:"sequence_range"`
}

// Transition identifies the two clips around a cut. Head is source material
// before the incoming clip's cut frame; Tail is material after the outgoing
// clip's cut frame.
type Transition struct {
	ID          string `json:"id"`
	LeftClipID  string `json:"left_clip_id"`
	RightClipID string `json:"right_clip_id"`
	Cut         int64  `json:"cut"`
	Head        int64  `json:"head_frames"`
	Tail        int64  `json:"tail_frames"`
}

// Duration returns the total overlap represented by both handle sides.
func (t Transition) Duration() int64 {
	return t.Head + t.Tail
}

// SymmetricTransition builds a frame-exact cross-dissolve. Odd durations give
// the extra frame to the outgoing/tail side.
func SymmetricTransition(id, leftID, rightID string, cut, duration int64) Transition {
	if duration < 0 {
		duration = 0
	}
	head := duration / 2
	return Transition{
		ID:          id,
		LeftClipID:  leftID,
		RightClipID: rightID,
		Cut:         cut,
		Head:        head,
		Tail:        duration - head,
	}
}

// Project is the smallest timeline document needed by the audit and repair.
type Project struct {
	SequenceBounds Range        `json:"sequence_bounds"`
	Clips          []Clip       `json:"clips"`
	Transitions    []Transition `json:"transitions"`
}

// Clip returns a pointer to the clip with id.
func (p *Project) Clip(id string) *Clip {
	if p == nil {
		return nil
	}
	for i := range p.Clips {
		if p.Clips[i].ID == id {
			return &p.Clips[i]
		}
	}
	return nil
}

// Clone returns a deep-ish copy; Range values and slices are independent.
func (p Project) Clone() Project {
	cp := p
	cp.Clips = append([]Clip(nil), p.Clips...)
	cp.Transitions = append([]Transition(nil), p.Transitions...)
	return cp
}

func (p Project) validate() error {
	seen := make(map[string]struct{}, len(p.Clips))
	if !p.SequenceBounds.valid() {
		return fmt.Errorf("sequence bounds %+v: %w", p.SequenceBounds, ErrOutOfBounds)
	}
	for i, clip := range p.Clips {
		if clip.ID == "" {
			return fmt.Errorf("clip[%d]: empty id", i)
		}
		if _, dup := seen[clip.ID]; dup {
			return fmt.Errorf("duplicate clip id %q", clip.ID)
		}
		seen[clip.ID] = struct{}{}
		if !clip.Media.valid() || clip.Media.Len() == 0 {
			return fmt.Errorf("clip %q: invalid media range %+v", clip.ID, clip.Media)
		}
		if !clip.Source.within(clip.Media) {
			return fmt.Errorf("clip %q: source %+v outside media %+v: %w",
				clip.ID, clip.Source, clip.Media, ErrOutOfBounds)
		}
		if !clip.Sequence.within(p.SequenceBounds) {
			return fmt.Errorf("clip %q: sequence %+v outside %+v: %w",
				clip.ID, clip.Sequence, p.SequenceBounds, ErrOutOfBounds)
		}
	}
	for i, tr := range p.Transitions {
		if tr.ID == "" {
			return fmt.Errorf("transition[%d]: empty id", i)
		}
		if tr.Head < 0 || tr.Tail < 0 {
			return fmt.Errorf("transition %q: negative handle length", tr.ID)
		}
		if p.Clip(tr.LeftClipID) == nil {
			return fmt.Errorf("transition %q: missing left clip %q", tr.ID, tr.LeftClipID)
		}
		if p.Clip(tr.RightClipID) == nil {
			return fmt.Errorf("transition %q: missing right clip %q", tr.ID, tr.RightClipID)
		}
	}
	return nil
}

var errMapping = errors.New("source/sequence mapping is not aligned")
