package sdh

import (
	"testing"
	"time"
)

func ms(v int) time.Duration { return time.Duration(v) * time.Millisecond }

func makeEntries(specs ...[3]int) []*Entry {
	entries := make([]*Entry, 0, len(specs))
	for i, s := range specs {
		entries = append(entries, &Entry{
			ID:    string(rune('a' + i)),
			Role:  RoleDialogue,
			Start: ms(s[0]),
			End:   ms(s[1]),
			Group: -1,
			Lane:  -1,
		})
	}
	return entries
}

func laneByID(entries []*Entry) map[string]int {
	out := map[string]int{}
	for _, e := range entries {
		out[e.ID] = e.Lane
	}
	return out
}

func TestOverlapLanesNonOverlapping(t *testing.T) {
	entries := makeEntries([3]int{0, 1000, 0}, [3]int{1000, 2000, 0})
	layout := ResolveOverlaps(entries, DefaultLayoutOptions())
	lanes := laneByID(entries)
	if lanes["a"] != 0 || lanes["b"] != 0 {
		t.Fatalf("touching/non-overlap lanes = %v", lanes)
	}
	if len(layout.Groups) != 0 {
		t.Fatalf("expected no overlap groups, got %d", len(layout.Groups))
	}
}

func TestOverlapLanesStackAndReuse(t *testing.T) {
	// a [0,4] and b [1,5] overlap; c [5,6] starts when b ends and reuses.
	entries := makeEntries(
		[3]int{0, 4000, 0},
		[3]int{1000, 5000, 0},
		[3]int{5000, 6000, 0},
	)
	layout := ResolveOverlaps(entries, DefaultLayoutOptions())
	lanes := laneByID(entries)
	if lanes["a"] != 0 || lanes["b"] != 1 {
		t.Fatalf("overlap lanes = %v, want a0 b1", lanes)
	}
	if lanes["c"] != 0 && lanes["c"] != 1 {
		t.Fatalf("c should reuse a free lane, got %d", lanes["c"])
	}
	if len(layout.Groups) != 1 || len(layout.Groups[0].EntryIDs) != 2 {
		t.Fatalf("groups = %+v", layout.Groups)
	}
	if entries[0].Group < 0 || entries[1].Group < 0 || entries[2].Group != -1 {
		t.Fatalf("group assignment wrong: %d %d %d", entries[0].Group, entries[1].Group, entries[2].Group)
	}
}

func TestThreeWayOverlap(t *testing.T) {
	entries := makeEntries(
		[3]int{0, 3000, 0},
		[3]int{500, 3500, 0},
		[3]int{1000, 4000, 0},
	)
	layout := ResolveOverlaps(entries, DefaultLayoutOptions())
	lanes := laneByID(entries)
	used := map[int]bool{}
	for _, lane := range lanes {
		if used[lane] {
			t.Fatalf("lane %d reused by overlapping entries: %v", lane, lanes)
		}
		used[lane] = true
	}
	if len(used) != 3 {
		t.Fatalf("expected 3 lanes, got %v", lanes)
	}
	if layout.MaxLanes[AnchorBottom] != 3 {
		t.Fatalf("MaxLanes bottom = %d", layout.MaxLanes[AnchorBottom])
	}
}

func TestOverlapDeterministic(t *testing.T) {
	entry := func(id string, s, e int) *Entry {
		return &Entry{ID: id, Role: RoleDialogue, Start: ms(s), End: ms(e), Group: -1, Lane: -1}
	}
	first := []*Entry{entry("a", 0, 4000), entry("b", 1000, 5000), entry("c", 2000, 6000)}
	second := []*Entry{entry("c", 2000, 6000), entry("a", 0, 4000), entry("b", 1000, 5000)}
	ResolveOverlaps(first, DefaultLayoutOptions())
	ResolveOverlaps(second, DefaultLayoutOptions())
	l1, l2 := laneByID(first), laneByID(second)
	if l1["a"] != l2["a"] || l1["b"] != l2["b"] || l1["c"] != l2["c"] {
		t.Fatalf("non-deterministic lanes: %v vs %v", l1, l2)
	}
}

func TestMaxLanesOverflowReported(t *testing.T) {
	entries := makeEntries([3]int{0, 3000, 0}, [3]int{500, 3500, 0}, [3]int{1000, 4000, 0})
	layout := ResolveOverlaps(entries, LayoutOptions{MaxLanes: 2})
	if len(layout.Overflow) != 1 || layout.Overflow[0] != "c" {
		t.Fatalf("overflow = %v, want [c]", layout.Overflow)
	}
	// Lane still assigned beyond cap so nothing is dropped.
	if entries[2].Lane != 2 {
		t.Fatalf("overflow entry lane = %d, want 2", entries[2].Lane)
	}
}

func TestTopAndBottomBandsLaneIndependently(t *testing.T) {
	bottom := &Entry{ID: "d", Role: RoleDialogue, Start: 0, End: 2000}
	top := &Entry{ID: "s", Role: RoleSound, Start: 0, End: 2000}
	layout := ResolveOverlaps([]*Entry{bottom, top}, DefaultLayoutOptions())
	if bottom.Lane != 0 || top.Lane != 0 {
		t.Fatalf("roles on different anchors should both use lane 0: d=%d s=%d", bottom.Lane, top.Lane)
	}
	if layout.MaxLanes[AnchorTop] != 1 || layout.MaxLanes[AnchorBottom] != 1 {
		t.Fatalf("max lanes = %+v", layout.MaxLanes)
	}
}
