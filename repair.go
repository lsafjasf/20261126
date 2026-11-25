package handlefix

import (
	"fmt"
	"sort"
)

// PrepareExport returns a copy whose source and sequence ranges cover every
// transition without leaving media or sequence bounds. The input project is
// never modified.
//
// The root-cause repair is deliberately small: transition prep must not trim
// the incoming Source.Start rightward or outgoing Source.End leftward. It
// restores those edges from the opposite stable anchor and then extends both
// source and sequence spaces by the same number of handle frames.
func PrepareExport(project Project) (Project, *AuditReport, error) {
	if err := project.validate(); err != nil {
		return Project{}, nil, err
	}

	out := project.Clone()
	ordered := append([]Transition(nil), out.Transitions...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Cut != ordered[j].Cut {
			return ordered[i].Cut < ordered[j].Cut
		}
		return ordered[i].ID < ordered[j].ID
	})

	for _, tr := range ordered {
		left := out.Clip(tr.LeftClipID)
		right := out.Clip(tr.RightClipID)
		if left == nil || right == nil {
			return Project{}, nil, fmt.Errorf("transition %q references missing clip", tr.ID)
		}

		if left.Source.Len() != left.Sequence.Len() {
			return Project{}, nil, fmt.Errorf("transition %q outgoing clip %q: %w",
				tr.ID, left.ID, errMapping)
		}
		if right.Source.Len() != right.Sequence.Len() {
			return Project{}, nil, fmt.Errorf("transition %q incoming clip %q: %w",
				tr.ID, right.ID, errMapping)
		}

		if err := extendOutgoingHandle(left, tr, out.SequenceBounds); err != nil {
			return Project{}, nil, fmt.Errorf("transition %q: %w", tr.ID, err)
		}
		if err := extendIncomingHandle(right, tr, out.SequenceBounds); err != nil {
			return Project{}, nil, fmt.Errorf("transition %q: %w", tr.ID, err)
		}
	}

	if err := out.validate(); err != nil {
		return Project{}, nil, err
	}
	report, err := Audit(out)
	if err != nil {
		return Project{}, nil, err
	}
	return out, report, nil
}

func extendOutgoingHandle(clip *Clip, tr Transition, seqBounds Range) error {
	sourceCut := clip.Source.Start + (tr.Cut - clip.Sequence.Start)
	sourceRoom := max64(0, clip.Media.End-sourceCut)
	seqRoom := max64(0, seqBounds.End-tr.Cut)
	frames := min64(tr.Tail, min64(sourceRoom, seqRoom))

	targetSeqEnd := max64(clip.Sequence.End, tr.Cut+frames)
	delta := targetSeqEnd - clip.Sequence.End
	if delta <= 0 {
		return nil
	}
	if clip.Source.End+delta > clip.Media.End {
		return fmt.Errorf("clip %q outgoing extension would cross media end: %w", clip.ID, ErrOutOfBounds)
	}
	clip.Sequence.End += delta
	clip.Source.End += delta
	return nil
}

func extendIncomingHandle(clip *Clip, tr Transition, seqBounds Range) error {
	sourceCut := clip.Source.End - (clip.Sequence.End - tr.Cut)
	sourceRoom := max64(0, sourceCut-clip.Media.Start)
	seqRoom := max64(0, tr.Cut-seqBounds.Start)
	frames := min64(tr.Head, min64(sourceRoom, seqRoom))

	targetSeqStart := min64(clip.Sequence.Start, tr.Cut-frames)
	delta := clip.Sequence.Start - targetSeqStart
	if delta <= 0 {
		return nil
	}
	if clip.Source.Start-delta < clip.Media.Start {
		return fmt.Errorf("clip %q incoming extension would cross media start: %w", clip.ID, ErrOutOfBounds)
	}
	clip.Sequence.Start -= delta
	clip.Source.Start -= delta
	return nil
}

// ReverseEdits restores edges recorded by a trim trace. Edits are reversed in
// reverse chronological order so repeated movements of one edge are undone
// exactly. The returned project is validated against media bounds.
func ReverseEdits(project Project, edits []EdgeEdit) (Project, error) {
	out := project.Clone()
	byID := make(map[string]*Clip, len(out.Clips))
	for i := range out.Clips {
		byID[out.Clips[i].ID] = &out.Clips[i]
	}

	for i := len(edits) - 1; i >= 0; i-- {
		edit := edits[i]
		clip := byID[edit.ClipID]
		if clip == nil {
			return Project{}, fmt.Errorf("trim trace references missing clip %q", edit.ClipID)
		}
		switch edit.Edge {
		case EdgeSourceStart:
			clip.Source.Start = edit.Before
		case EdgeSourceEnd:
			clip.Source.End = edit.Before
		default:
			return Project{}, fmt.Errorf("clip %q: unknown edge %q", edit.ClipID, edit.Edge)
		}
		if !clip.Source.within(clip.Media) {
			return Project{}, fmt.Errorf("clip %q: reversed trim %+v gives source %+v outside media: %w",
				clip.ID, edit, clip.Source, ErrOutOfBounds)
		}
	}

	if err := out.validate(); err != nil {
		return Project{}, err
	}
	return out, nil
}
