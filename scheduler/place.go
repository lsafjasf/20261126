package scheduler

import (
	"fmt"
	"sort"
)

type selection struct {
	variant  string
	duration int64
	startMS  int64
	endMS    int64
}

func choosePlacement(cue preparedCue, constraints []Constraint, clearance int64) (selection, bool) {
	type candidateVariant struct {
		name    string
		variant *Variant
	}
	candidates := []candidateVariant{
		{name: "long", variant: cue.long},
		{name: "short", variant: cue.short},
	}
	for _, candidate := range candidates {
		if candidate.variant == nil {
			continue
		}
		duration, _ := variantDuration(*candidate.variant)
		if start, ok := bestStart(cue.window, duration, cue.cue.AnchorMS, constraints, clearance); ok {
			return selection{
				variant:  candidate.name,
				duration: duration,
				startMS:  start,
				endMS:    start + duration,
			}, true
		}
	}
	return selection{}, false
}

func bestStart(window Window, duration, anchor int64, constraints []Constraint, clearance int64) (int64, bool) {
	gaps := freeGaps(window, constraints, clearance)
	best := int64(0)
	found := false
	bestContainsAnchor := false
	bestDistance := int64(0)
	for _, gap := range gaps {
		latest := gap.EndMS - duration
		if latest < gap.StartMS {
			continue
		}
		for _, start := range candidateStarts(gap, duration, anchor, latest) {
			end := start + duration
			containsAnchor := start <= anchor && anchor <= end
			distance := centerDistance(start, end, anchor)
			if !found || betterStart(
				containsAnchor, distance, start, anchor,
				bestContainsAnchor, bestDistance, best,
			) {
				best = start
				bestContainsAnchor = containsAnchor
				bestDistance = distance
				found = true
			}
		}
	}
	return best, found
}

func centerDistance(start, end, point int64) int64 {
	center := start + (end-start)/2
	if center < point {
		return point - center
	}
	return center - point
}

func candidateStarts(gap Window, duration, anchor, latest int64) []int64 {
	raw := []int64{
		gap.StartMS,
		latest,
		anchor,
		anchor - duration,
		anchor - duration/2,
	}
	seen := make(map[int64]struct{}, len(raw))
	starts := make([]int64, 0, len(raw))
	for _, start := range raw {
		if start < gap.StartMS {
			start = gap.StartMS
		}
		if start > latest {
			start = latest
		}
		if _, exists := seen[start]; exists {
			continue
		}
		seen[start] = struct{}{}
		starts = append(starts, start)
	}
	return starts
}

func betterStart(
	containsAnchor bool, distance, start, anchor int64,
	bestContainsAnchor bool, bestDistance, bestStart int64,
) bool {
	if containsAnchor != bestContainsAnchor {
		return containsAnchor
	}
	if distance != bestDistance {
		return distance < bestDistance
	}
	if (start >= anchor) != (bestStart >= anchor) {
		return start >= anchor
	}
	return start < bestStart
}

func intervalDistance(start, end, point int64) int64 {
	switch {
	case point < start:
		return start - point
	case point > end:
		return point - end
	default:
		return 0
	}
}

func scheduledConstraint(cueID, variant string, selected selection, existing []Constraint) Constraint {
	base := "ad:" + cueID
	id := base
	used := make(map[string]struct{}, len(existing))
	for _, constraint := range existing {
		used[constraint.ID] = struct{}{}
	}
	for suffix := 2; ; suffix++ {
		if _, exists := used[id]; !exists {
			break
		}
		id = fmt.Sprintf("%s#%d", base, suffix)
	}
	return Constraint{
		ID:      id,
		Kind:    ConstraintAudioDesc,
		StartMS: selected.startMS,
		EndMS:   selected.endMS,
		Label:   cueID,
		CueID:   cueID,
		Variant: variant,
	}
}

func cueScheduleOrder(cues []preparedCue) []preparedCue {
	ordered := append([]preparedCue(nil), cues...)
	sort.SliceStable(ordered, func(i, j int) bool {
		a, b := ordered[i].cue, ordered[j].cue
		if a.Priority != b.Priority {
			return a.Priority > b.Priority
		}
		if a.AnchorMS != b.AnchorMS {
			return a.AnchorMS < b.AnchorMS
		}
		return a.ID < b.ID
	})
	return ordered
}
