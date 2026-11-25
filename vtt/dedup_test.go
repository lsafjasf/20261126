package vtt

import "testing"

func cueWith(id string, start, end int64, text string) *Cue {
	return &Cue{ID: id, Start: start, End: end, Text: text}
}

func TestIsDuplicateThreeLayers(t *testing.T) {
	base := cueWith("1", 1000, 2000, "hello")
	same := cueWith("1", 1000, 2000, "hello")

	if !IsDuplicate(base, same) {
		t.Error("identical three-layer cues must be duplicates")
	}

	differentTime := cueWith("1", 3000, 4000, "hello")
	differentText := cueWith("1", 1000, 2000, "world")
	differentID := cueWith("2", 1000, 2000, "hello")

	for name, other := range map[string]*Cue{
		"different time": differentTime,
		"different text": differentText,
		"different id":   differentID,
	} {
		if IsDuplicate(base, other) {
			t.Errorf("%s must not count as duplicate", name)
		}
	}
}

func TestDeduplicateKeepsSameNameDifferentContent(t *testing.T) {
	doc := &Document{Blocks: []Block{
		cueWith("1", 1000, 2000, "first"),
		cueWith("1", 3000, 4000, "second"),
		cueWith("1", 5000, 6000, "third"),
	}}
	report := doc.Deduplicate()
	if report.Removed != 0 {
		t.Fatalf("removed %d cues, want 0", report.Removed)
	}
	if len(doc.Cues()) != 3 {
		t.Fatalf("kept %d cues, want 3", len(doc.Cues()))
	}
	if report.RepeatedIDs["1"] != 3 {
		t.Errorf("RepeatedIDs = %v, want 1->3", report.RepeatedIDs)
	}
}

func TestDeduplicateRemovesTrueDuplicates(t *testing.T) {
	doc := &Document{Blocks: []Block{
		cueWith("1", 1000, 2000, "same"),
		cueWith("1", 1000, 2000, "same"),
		cueWith("", 5000, 6000, "anonymous"),
		cueWith("", 5000, 6000, "anonymous"),
		cueWith("2", 7000, 8000, "unique"),
	}}
	report := doc.Deduplicate()
	if report.Removed != 2 {
		t.Fatalf("removed %d, want 2", report.Removed)
	}
	if report.Kept != 3 || len(doc.Cues()) != 3 {
		t.Fatalf("kept %d cues, want 3", report.Kept)
	}
	if len(report.RemovedIDs) != 2 || report.RemovedIDs[0] != "1" || report.RemovedIDs[1] != "" {
		t.Errorf("RemovedIDs = %v", report.RemovedIDs)
	}
}

func TestDeduplicatePreservesOrderAndNonCueBlocks(t *testing.T) {
	note := &Note{Text: "NOTE x"}
	style := &Style{Text: "::cue{}"}
	doc := &Document{Blocks: []Block{
		note,
		cueWith("1", 1000, 2000, "a"),
		style,
		cueWith("1", 1000, 2000, "a"),
		cueWith("2", 3000, 4000, "b"),
	}}
	doc.Deduplicate()
	if len(doc.Blocks) != 4 {
		t.Fatalf("blocks = %d, want 4", len(doc.Blocks))
	}
	if doc.Blocks[0] != note || doc.Blocks[2] != style {
		t.Error("non-cue blocks or order not preserved")
	}
}

func TestDeduplicateIdempotent(t *testing.T) {
	doc := &Document{Blocks: []Block{
		cueWith("1", 1000, 2000, "a"),
		cueWith("1", 1000, 2000, "a"),
		cueWith("1", 3000, 4000, "b"),
	}}
	first := doc.Deduplicate()
	second := doc.Deduplicate()
	if first.Removed != 1 || second.Removed != 0 {
		t.Errorf("runs = %+v, %+v", first, second)
	}
}

func TestIdentityIgnoresSettings(t *testing.T) {
	a := cueWith("1", 1000, 2000, "x")
	b := cueWith("1", 1000, 2000, "x")
	b.Settings = "line:50%"
	if a.Identity() != b.Identity() {
		t.Error("settings must not participate in cue identity")
	}
}
