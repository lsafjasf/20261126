package audiopkg

import (
	"encoding/json"
	"testing"
)

// snapshotFields maps track ID to its JSON encoding, so tests can prove
// that reordering never alters any field of any track.
func snapshotFields(t *testing.T, p *Package) map[string]string {
	t.Helper()
	snap := make(map[string]string, len(p.Tracks))
	for _, tr := range p.Tracks {
		data, err := json.Marshal(tr)
		if err != nil {
			t.Fatal(err)
		}
		snap[tr.ID] = string(data)
	}
	return snap
}

func TestReorderKeepsFieldsIntact(t *testing.T) {
	p := &Package{
		Name: "demo",
		Tracks: []Track{
			{ID: "zh-sdh", Language: "zh-Hans", Role: RoleAlternate, Accessibility: AccessibilityHearingImpaired, Title: "辅助"},
			{ID: "en-main", Language: "en", Role: RoleMain, Default: true, Title: "English"},
			{ID: "zh-main", Language: "zh-Hans", Role: RoleMain, Title: "普通话"},
			{ID: "en-commentary", Language: "en", Role: RoleCommentary, Title: "Commentary"},
		},
	}
	before := snapshotFields(t, p)
	p.SortCanonical()
	after := snapshotFields(t, p)
	if len(after) != len(before) {
		t.Fatalf("track count changed: %d -> %d", len(before), len(after))
	}
	for id, want := range before {
		if after[id] != want {
			t.Errorf("track %q fields changed by reorder:\n want %s\n got  %s", id, want, after[id])
		}
	}
	wantOrder := []string{"en-main", "en-commentary", "zh-main", "zh-sdh"}
	for i, id := range wantOrder {
		if p.Tracks[i].ID != id {
			t.Errorf("position %d: want %q, got %q", i, id, p.Tracks[i].ID)
		}
	}
}
func TestReorderCustomComparatorKeepsFields(t *testing.T) {
	p := validPackage()
	before := snapshotFields(t, p)
	p.Reorder(func(a, b *Track) bool { return a.ID > b.ID }) // reverse by ID
	after := snapshotFields(t, p)
	for id, want := range before {
		if after[id] != want {
			t.Errorf("track %q fields changed by reorder", id)
		}
	}
}
