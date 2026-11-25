package scheduler

import (
	"fmt"
	"sort"
	"strings"
)

func buildConstraints(timeline Timeline) ([]Constraint, error) {
	var constraints []Constraint
	seenIDs := make(map[string]struct{})

	add := func(kind ConstraintKind, intervals []Interval) error {
		for i, interval := range intervals {
			if _, err := interval.window(); err != nil {
				return fmt.Errorf("timeline.%s[%d]: %w", kind, i, err)
			}
			if timeline.DurationMS > 0 && interval.EndMS > timeline.DurationMS {
				return fmt.Errorf(
					"timeline.%s[%d]: end_ms %d exceeds timeline duration %d",
					kind, i, interval.EndMS, timeline.DurationMS,
				)
			}
			id := strings.TrimSpace(interval.ID)
			if id == "" {
				id = fmt.Sprintf("%s-%d", kind, i+1)
			}
			if _, exists := seenIDs[id]; exists {
				return fmt.Errorf("timeline: duplicate constraint id %q", id)
			}
			seenIDs[id] = struct{}{}
			label := intervalLabel(interval)
			if label == "" {
				label = id
			}
			constraints = append(constraints, Constraint{
				ID:      id,
				Kind:    kind,
				StartMS: interval.StartMS,
				EndMS:   interval.EndMS,
				Label:   label,
			})
		}
		return nil
	}

	if err := add(ConstraintDialogue, timeline.Dialogue); err != nil {
		return nil, err
	}
	if err := add(ConstraintSoundEffect, timeline.SoundEffects); err != nil {
		return nil, err
	}
	if err := add(ConstraintSubtitle, timeline.Subtitles); err != nil {
		return nil, err
	}
	if err := add(ConstraintAudioDesc, timeline.ExistingAudioDesc); err != nil {
		return nil, err
	}

	sortConstraints(constraints)
	return constraints, nil
}

func intervalLabel(interval Interval) string {
	if label := strings.TrimSpace(interval.Label); label != "" {
		return label
	}
	text := strings.TrimSpace(interval.Text)
	speaker := strings.TrimSpace(interval.Speaker)
	switch {
	case speaker != "" && text != "":
		return speaker + ": " + text
	case text != "":
		return text
	default:
		return interval.ID
	}
}

func sortConstraints(constraints []Constraint) {
	sort.SliceStable(constraints, func(i, j int) bool {
		a, b := constraints[i], constraints[j]
		if a.StartMS != b.StartMS {
			return a.StartMS < b.StartMS
		}
		if a.EndMS != b.EndMS {
			return a.EndMS < b.EndMS
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.ID < b.ID
	})
}

// freeGaps subtracts every merged constraint from window. Clearance expands
// both sides of each forbidden interval without changing the reported source.
func freeGaps(window Window, constraints []Constraint, clearance int64) []Window {
	blocked := make([]Window, 0, len(constraints))
	for _, constraint := range constraints {
		start := constraint.StartMS - clearance
		end := constraint.EndMS + clearance
		if end <= window.StartMS || start >= window.EndMS {
			continue
		}
		if start < window.StartMS {
			start = window.StartMS
		}
		if end > window.EndMS {
			end = window.EndMS
		}
		blocked = append(blocked, Window{StartMS: start, EndMS: end})
	}
	if len(blocked) == 0 {
		return []Window{window}
	}

	sort.Slice(blocked, func(i, j int) bool {
		if blocked[i].StartMS != blocked[j].StartMS {
			return blocked[i].StartMS < blocked[j].StartMS
		}
		return blocked[i].EndMS < blocked[j].EndMS
	})

	gaps := make([]Window, 0, len(blocked)+1)
	cursor := window.StartMS
	for _, interval := range blocked {
		if interval.StartMS > cursor {
			gaps = append(gaps, Window{StartMS: cursor, EndMS: interval.StartMS})
		}
		if interval.EndMS > cursor {
			cursor = interval.EndMS
		}
	}
	if cursor < window.EndMS {
		gaps = append(gaps, Window{StartMS: cursor, EndMS: window.EndMS})
	}
	return gaps
}

func constraintsNear(window Window, constraints []Constraint, clearance int64) []Conflict {
	conflicts := make([]Conflict, 0)
	for _, constraint := range constraints {
		start := constraint.StartMS - clearance
		end := constraint.EndMS + clearance
		if end <= window.StartMS || start >= window.EndMS {
			continue
		}
		conflicts = append(conflicts, Conflict{
			ConstraintID: constraint.ID,
			Kind:         constraint.Kind,
			StartMS:      constraint.StartMS,
			EndMS:        constraint.EndMS,
			Label:        constraint.Label,
			OverlapMS:    overlapMS(window.StartMS, window.EndMS, constraint.StartMS, constraint.EndMS),
		})
	}
	return conflicts
}

func overlapMS(aStart, aEnd, bStart, bEnd int64) int64 {
	start := max(aStart, bStart)
	end := min(aEnd, bEnd)
	if end <= start {
		return 0
	}
	return end - start
}
