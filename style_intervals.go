package ttmlevents

import (
	"sort"
	"strings"
	"time"
)

func buildStyleIntervals(runs []TextRun, slices []Slice, tracks []AnimationTrack) []StyleInterval {
	result := make([]StyleInterval, 0)

	for _, run := range runs {
		if run.Whitespace || run.Kind != "text" {
			continue
		}
		lanes := make(map[string][]StyleInterval)
		propertyOrder := make([]string, 0)

		for _, slice := range slices {
			if !runActiveAt(run, slice.Begin) {
				continue
			}
			effective := effectiveStyleForRun(run, slice.Begin, slice.End, tracks)
			for _, prop := range effective {
				if _, exists := lanes[prop.Name]; !exists {
					propertyOrder = append(propertyOrder, prop.Name)
				}
				interval := StyleInterval{
					RunID:         run.ID,
					Text:          strings.TrimSpace(run.Text),
					NodePath:      run.ElementPath,
					Property:      prop.Name,
					Begin:         slice.Begin,
					BeginSeconds:  durationSeconds(slice.Begin),
					End:           cloneDurationPtr(slice.End),
					OpenEnded:     slice.End == nil,
					Value:         prop.Value,
					EndValue:      prop.EndValue,
					Interpolation: prop.Interpolation,
					Source:        prop.Source,
					AnimationPath: prop.AnimationPath,
					MergeGroup:    prop.MergeGroup,
				}
				if slice.End != nil {
					sec := durationSeconds(*slice.End)
					interval.EndSeconds = &sec
				}
				lanes[prop.Name] = append(lanes[prop.Name], interval)
			}
		}

		sort.Strings(propertyOrder)
		for _, name := range propertyOrder {
			merged := mergeStyleLane(lanes[name])
			result = append(result, merged...)
		}
	}
	return result
}

func mergeStyleLane(in []StyleInterval) []StyleInterval {
	out := make([]StyleInterval, 0, len(in))
	for _, interval := range in {
		if len(out) == 0 {
			out = append(out, interval)
			continue
		}
		last := &out[len(out)-1]
		if canMergeStyleInterval(*last, interval) {
			last.End = cloneDurationPtr(interval.End)
			last.EndSeconds = cloneFloatPtr(interval.EndSeconds)
			last.OpenEnded = interval.OpenEnded
			if interval.EndValue != "" {
				last.EndValue = interval.EndValue
			}
			continue
		}
		out = append(out, interval)
	}
	return out
}

func canMergeStyleInterval(a, b StyleInterval) bool {
	if a.RunID != b.RunID || a.Property != b.Property || a.MergeGroup != b.MergeGroup {
		return false
	}
	if a.Source != b.Source || a.AnimationPath != b.AnimationPath || a.Interpolation != b.Interpolation {
		return false
	}
	if !sameTimeEndpoint(a.End, b.Begin) {
		return false
	}
	switch a.Interpolation {
	case "linear", "spline":
		return a.EndValue == b.Value
	default:
		return a.Value == b.Value
	}
}

func sameTimeEndpoint(end *time.Duration, begin time.Duration) bool {
	return end != nil && *end == begin
}

func cloneFloatPtr(v *float64) *float64 {
	if v == nil {
		return nil
	}
	x := *v
	return &x
}
