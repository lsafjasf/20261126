package handlefix

import (
	"sort"
)

// SideReport explains one adjacent clip's transition handle.
type SideReport struct {
	ClipID           string   `json:"clip_id"`
	Side             string   `json:"side"`
	TrimStep         string   `json:"trim_step,omitempty"`
	RequiredFrames   int64    `json:"required_frames"`
	AvailableFrames  int64    `json:"available_frames"`
	EffectiveFrames  int64    `json:"effective_frames"`
	DeficitFrames    int64    `json:"range_deficit_frames"`
	SourceDuration   int64    `json:"source_duration"`
	SequenceDuration int64    `json:"sequence_duration"`
	DurationDelta    int64    `json:"duration_delta"`
	SequenceLimit    int64    `json:"sequence_limit"`
	SourceCutFrame   int64    `json:"source_cut_frame"`
	SourceBefore     Range    `json:"source_before"`
	SequenceRange    Range    `json:"sequence_range"`
	SourceSequenceOK bool     `json:"source_sequence_aligned"`
	CutOK            bool     `json:"cut_in_sequence_range"`
	InBounds         bool     `json:"in_bounds"`
	DeficitAmbiguous bool     `json:"deficit_ambiguous,omitempty"`
	Problems         []string `json:"problems,omitempty"`
}

// TransitionReport is the per-transition result of traversal.
type TransitionReport struct {
	Index         int        `json:"index"`
	TransitionID  string     `json:"transition_id"`
	Cut           int64      `json:"cut"`
	RequestedHead int64      `json:"requested_head_frames"`
	RequestedTail int64      `json:"requested_tail_frames"`
	EffectiveHead int64      `json:"effective_head_frames"`
	EffectiveTail int64      `json:"effective_tail_frames"`
	MappingOK     bool       `json:"mapping_ok"`
	HardCut       bool       `json:"hard_cut"`
	Damaged       bool       `json:"damaged"`
	Left          SideReport `json:"left_outgoing"`
	Right         SideReport `json:"right_incoming"`
	Problems      []string   `json:"problems,omitempty"`
}

// AuditReport contains every transition, preserving traversal order.
type AuditReport struct {
	Transitions  []TransitionReport `json:"transitions"`
	Locations    []DamageLocation   `json:"confirmed_damage,omitempty"`
	Total        int                `json:"total"`
	DamagedCount int                `json:"damaged_count"`
	HardCutCount int                `json:"hard_cut_count"`
}

// Audit traverses all transitions and compares source and sequence ranges.
// It does not mutate p. Range deficits identify inconsistent edges; use
// AuditTrace when a trim trace is available to name the exact step.
func Audit(p Project) (*AuditReport, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}

	ordered := append([]Transition(nil), p.Transitions...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Cut != ordered[j].Cut {
			return ordered[i].Cut < ordered[j].Cut
		}
		return ordered[i].ID < ordered[j].ID
	})

	report := &AuditReport{Transitions: make([]TransitionReport, 0, len(ordered))}
	for order, tr := range ordered {
		left := p.Clip(tr.LeftClipID)
		right := p.Clip(tr.RightClipID)
		item := TransitionReport{
			Index:         order,
			TransitionID:  tr.ID,
			Cut:           tr.Cut,
			RequestedHead: tr.Head,
			RequestedTail: tr.Tail,
		}

		item.Left = auditOutgoing(left, tr, p.SequenceBounds)
		item.Right = auditIncoming(right, tr, p.SequenceBounds)
		item.Problems = append(item.Problems, item.Left.Problems...)
		item.Problems = append(item.Problems, item.Right.Problems...)

		item.Damaged = item.Left.DeficitFrames > 0 || item.Right.DeficitFrames > 0
		mappingOK := item.Left.SourceSequenceOK && item.Right.SourceSequenceOK &&
			item.Left.CutOK && item.Right.CutOK
		item.MappingOK = mappingOK
		potential := item.Left.EffectiveFrames + item.Right.EffectiveFrames
		if mappingOK {
			item.EffectiveTail = item.Left.EffectiveFrames
			item.EffectiveHead = item.Right.EffectiveFrames
		}
		item.HardCut = !mappingOK || potential == 0
		if item.Damaged {
			report.DamagedCount++
		}
		if item.HardCut {
			report.HardCutCount++
		}
		report.Transitions = append(report.Transitions, item)
	}
	report.Total = len(report.Transitions)
	return report, nil
}

// AuditTrace combines range evidence with a trim trace. The trace confirms
// which named step moved the exact transition-facing source edge.
func AuditTrace(project Project, edits []EdgeEdit) (*AuditReport, error) {
	report, err := Audit(project)
	if err != nil {
		return nil, err
	}
	report.Locations = LocateDamage(project, edits)
	return report, nil
}

func auditOutgoing(clip *Clip, tr Transition, seqBounds Range) SideReport {
	side := SideReport{
		ClipID:         tr.LeftClipID,
		Side:           "outgoing_tail",
		RequiredFrames: tr.Tail,
		SequenceRange:  Range{},
		SourceBefore:   Range{},
		Problems:       []string{},
	}
	if clip == nil {
		side.Problems = append(side.Problems, "missing outgoing clip")
		return side
	}
	side.SequenceRange = clip.Sequence
	side.SourceBefore = clip.Source
	side.CutOK = tr.Cut >= clip.Sequence.Start && tr.Cut <= clip.Sequence.End
	if !side.CutOK {
		side.Problems = append(side.Problems, "cut is outside outgoing sequence range")
	}

	// Anchor from source start: an end trim changes Source.End only.
	sourceCut := clip.Source.Start + (tr.Cut - clip.Sequence.Start)
	side.SourceCutFrame = sourceCut
	expectedEnd := clip.Source.Start + clip.Sequence.Len()
	side.SourceDuration = clip.Source.Len()
	side.SequenceDuration = clip.Sequence.Len()
	side.DurationDelta = side.SourceDuration - side.SequenceDuration
	side.DeficitFrames = max64(0, expectedEnd-clip.Source.End)
	side.SourceSequenceOK = clip.Source.Len() == clip.Sequence.Len()
	side.DeficitAmbiguous = side.DeficitFrames > 0 && !side.SourceSequenceOK
	if !side.SourceSequenceOK {
		side.Problems = append(side.Problems, "source and sequence durations differ")
	}
	if side.DeficitFrames > 0 {
		side.TrimStep = StepHandleTrim
		side.Problems = append(side.Problems, "inward end trim removed outgoing handle")
	}

	sourceAvailable := max64(0, clip.Media.End-sourceCut)
	seqAvailable := max64(0, seqBounds.End-tr.Cut)
	side.SequenceLimit = seqAvailable
	side.AvailableFrames = min64(sourceAvailable, seqAvailable)
	side.EffectiveFrames = min64(tr.Tail, side.AvailableFrames)
	side.InBounds = sourceCut >= clip.Media.Start && sourceCut <= clip.Media.End
	if side.EffectiveFrames < tr.Tail {
		side.Problems = append(side.Problems, "not enough outgoing tail media")
	}
	return side
}

func auditIncoming(clip *Clip, tr Transition, seqBounds Range) SideReport {
	side := SideReport{
		ClipID:         tr.RightClipID,
		Side:           "incoming_head",
		RequiredFrames: tr.Head,
		Problems:       []string{},
	}
	if clip == nil {
		side.Problems = append(side.Problems, "missing incoming clip")
		return side
	}
	side.SequenceRange = clip.Sequence
	side.SourceBefore = clip.Source
	side.CutOK = tr.Cut >= clip.Sequence.Start && tr.Cut <= clip.Sequence.End
	if !side.CutOK {
		side.Problems = append(side.Problems, "cut is outside incoming sequence range")
	}

	// Anchor from source end: a start trim changes Source.Start only.
	sourceCut := clip.Source.End - (clip.Sequence.End - tr.Cut)
	side.SourceCutFrame = sourceCut
	expectedStart := clip.Source.End - clip.Sequence.Len()
	side.SourceDuration = clip.Source.Len()
	side.SequenceDuration = clip.Sequence.Len()
	side.DurationDelta = side.SourceDuration - side.SequenceDuration
	side.DeficitFrames = max64(0, clip.Source.Start-expectedStart)
	side.SourceSequenceOK = clip.Source.Len() == clip.Sequence.Len()
	side.DeficitAmbiguous = side.DeficitFrames > 0 && !side.SourceSequenceOK
	if !side.SourceSequenceOK {
		side.Problems = append(side.Problems, "source and sequence durations differ")
	}
	if side.DeficitFrames > 0 {
		side.TrimStep = StepHandleTrim
		side.Problems = append(side.Problems, "inward start trim removed incoming handle")
	}

	sourceAvailable := max64(0, sourceCut-clip.Media.Start)
	seqAvailable := max64(0, tr.Cut-seqBounds.Start)
	side.SequenceLimit = seqAvailable
	side.AvailableFrames = min64(sourceAvailable, seqAvailable)
	side.EffectiveFrames = min64(tr.Head, side.AvailableFrames)
	side.InBounds = sourceCut >= clip.Media.Start && sourceCut <= clip.Media.End
	if side.EffectiveFrames < tr.Head {
		side.Problems = append(side.Problems, "not enough incoming head media")
	}
	return side
}
