package scheduler

type outcome struct {
	placement *Placement
	omission  *Omission
}

// Schedule builds one merged constraint set and places every cue that can fit.
// Higher-priority cues are placed first; long versions are preferred whenever a
// conflict-free slot exists.
func Schedule(input Plan) (*Result, error) {
	plan, err := prepare(input)
	if err != nil {
		return nil, err
	}
	constraints, err := buildConstraints(plan.timeline)
	if err != nil {
		return nil, err
	}

	outcomes := make(map[string]outcome, len(plan.cues))
	for _, cue := range cueScheduleOrder(plan.cues) {
		selected, ok := choosePlacement(cue, constraints, plan.options.MinClearanceMS)
		conflicts := constraintsNear(cue.window, constraints, plan.options.MinClearanceMS)
		if ok {
			placement := Placement{
				CueID:       cue.cue.ID,
				Variant:     selected.variant,
				StartMS:     selected.startMS,
				EndMS:       selected.endMS,
				AnchorMS:    cue.cue.AnchorMS,
				Explanation: placementExplanation(cue, selected, conflicts),
			}
			if selected.variant == "short" {
				placement.DroppedInfo = droppedByShort(cue.cue)
				if cue.long != nil {
					placement.Conflicts = conflicts
				}
			}
			outcomes[cue.cue.ID] = outcome{placement: &placement}
			constraints = append(constraints, scheduledConstraint(
				cue.cue.ID, selected.variant, selected, constraints,
			))
			continue
		}

		omission := Omission{
			CueID:       cue.cue.ID,
			AnchorMS:    cue.cue.AnchorMS,
			Reason:      "no_conflict_free_slot",
			DroppedInfo: droppedByOmission(cue.cue),
			Conflicts:   conflicts,
			Explanation: omissionExplanation(cue, conflicts, plan.options.MinClearanceMS, constraints),
		}
		outcomes[cue.cue.ID] = outcome{omission: &omission}
	}

	sortConstraints(constraints)
	result := &Result{
		SchemaVersion: SchemaVersion,
		Placements:    make([]Placement, 0, len(plan.cues)),
		Constraints:   constraints,
	}
	for _, cue := range plan.cues {
		scheduled := outcomes[cue.cue.ID]
		if scheduled.placement != nil {
			result.Placements = append(result.Placements, *scheduled.placement)
		}
		if scheduled.omission != nil {
			result.Omitted = append(result.Omitted, *scheduled.omission)
		}
	}
	result.Summary = summarize(plan.cues, result)
	return result, nil
}

func summarize(cues []preparedCue, result *Result) Summary {
	counts := map[string]int{
		string(ConstraintDialogue):    0,
		string(ConstraintSoundEffect): 0,
		string(ConstraintSubtitle):    0,
		string(ConstraintAudioDesc):   0,
	}
	for _, constraint := range result.Constraints {
		counts[string(constraint.Kind)]++
	}
	summary := Summary{
		Cues:             len(cues),
		Omitted:          len(result.Omitted),
		Constraints:      len(result.Constraints),
		ConstraintCounts: counts,
	}
	for _, placement := range result.Placements {
		switch placement.Variant {
		case "long":
			summary.LongPlaced++
		case "short":
			summary.ShortPlaced++
		}
	}
	return summary
}
