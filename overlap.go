package sdh

import (
	"sort"
	"time"
)

// IntervalsOverlap reports whether [aStart,aEnd) and [bStart,bEnd) share a
// strict time range. Intervals that only touch at an endpoint do not overlap,
// so one cue may start exactly when another ends without stacking.
func IntervalsOverlap(aStart, aEnd, bStart, bEnd time.Duration) bool {
	return aStart < bEnd && bStart < aEnd
}

// Overlaps reports whether two entries overlap on the timeline.
func (e *Entry) Overlaps(other *Entry) bool {
	if e == nil || other == nil {
		return false
	}
	return IntervalsOverlap(e.Start, e.End, other.Start, other.End)
}

// ResolveOverlaps assigns deterministic overlap groups and visual lanes to
// entries. Lanes are allocated independently for the top and bottom anchor
// bands of each role. The entries' Group and Lane fields are updated.
func ResolveOverlaps(entries []*Entry, opts LayoutOptions) *Layout {
	layout := &Layout{MaxLanes: map[Anchor]int{}}

	for _, entry := range entries {
		entry.Group = -1
		entry.Lane = -1
	}
	if len(entries) == 0 {
		return layout
	}

	sorted := append([]*Entry(nil), entries...)
	sort.SliceStable(sorted, func(i, j int) bool {
		a, b := sorted[i], sorted[j]
		if a.Start != b.Start {
			return a.Start < b.Start
		}
		if a.End != b.End {
			return a.End > b.End
		}
		if roleOrder(a.Role) != roleOrder(b.Role) {
			return roleOrder(a.Role) < roleOrder(b.Role)
		}
		return a.ID < b.ID
	})

	// Sweep interval connected components. A sorted entry joins the open
	// component while it starts before the component's maximum end.
	type component struct {
		entries []*Entry
		end     time.Duration
	}
	var components []*component
	var current *component
	for _, entry := range sorted {
		if current == nil || entry.Start >= current.end {
			current = &component{end: entry.End}
			components = append(components, current)
		} else if entry.End > current.end {
			current.end = entry.End
		}
		current.entries = append(current.entries, entry)
	}

	// Assign public group ids only to real overlaps (2+ entries).
	groupByEntry := map[string]int{}
	var groups []OverlapGroup
	for _, comp := range components {
		if len(comp.entries) < 2 {
			continue
		}
		gid := len(groups)
		group := OverlapGroup{ID: gid, Start: comp.entries[0].Start, End: comp.end}
		for _, entry := range comp.entries {
			entry.Group = gid
			groupByEntry[entry.ID] = gid
			group.EntryIDs = append(group.EntryIDs, entry.ID)
			if entry.Start < group.Start {
				group.Start = entry.Start
			}
			if entry.End > group.End {
				group.End = entry.End
			}
		}
		sort.Strings(group.EntryIDs)
		groups = append(groups, group)
	}
	layout.Groups = groups

	// Greedy first-fit lane assignment per anchor band.
	for _, anchor := range []Anchor{AnchorBottom, AnchorTop} {
		var laneEnds []time.Duration
		for _, entry := range sorted {
			if entry.Role.Anchor() != anchor {
				continue
			}
			lane := -1
			for i, end := range laneEnds {
				if end <= entry.Start {
					lane = i
					laneEnds[i] = entry.End
					break
				}
			}
			if lane == -1 {
				lane = len(laneEnds)
				laneEnds = append(laneEnds, entry.End)
			}
			entry.Lane = lane
			if gid, ok := groupByEntry[entry.ID]; ok {
				if lane > groups[gid].MaxLane {
					groups[gid].MaxLane = lane
				}
			}
		}
		layout.MaxLanes[anchor] = len(laneEnds)
	}

	if opts.MaxLanes > 0 {
		for _, entry := range sorted {
			if entry.Lane >= opts.MaxLanes {
				layout.Overflow = append(layout.Overflow, entry.ID)
			}
		}
		sort.Strings(layout.Overflow)
	}
	return layout
}

// HasOverlap reports whether the project contains any temporal overlap group.
func (p *Project) HasOverlap() bool {
	return p.Layout != nil && len(p.Layout.Groups) > 0
}
