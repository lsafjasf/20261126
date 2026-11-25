package sdh

import "testing"

func TestSplitFixtureLayers(t *testing.T) {
	cues := loadFixtureCues(t)
	p := Split(cues)

	wantCounts := map[Role]int{
		RoleDialogue:  5, // cue1, cue5, cue9x2, cue10
		RoleVoiceOver: 2, // cue3 VO, cue7
		RoleLyric:     2, // cue3, cue6
		RoleSpeaker:   5, // 张三, JOHN, 李四, MARY, JOHN
		RoleSound:     4, // 雷声, door, 笑声, 敲门
		RoleMusic:     1, // jazz
	}
	for role, want := range wantCounts {
		if got := len(p.LayerEntries(role)); got != want {
			t.Errorf("layer %s count = %d, want %d", role, got, want)
		}
	}

	if len(p.LayerOrder) != len(wantCounts) {
		t.Fatalf("LayerOrder = %v, want all six roles", p.LayerOrder)
	}
	if p.Layout == nil || len(p.Layout.Groups) != 4 {
		t.Fatalf("overlap groups = %v, want 4", p.Layout)
	}
	if p.Layout.MaxLanes[AnchorTop] != 2 {
		t.Fatalf("top lanes = %d, want 2 (two simultaneous sounds)", p.Layout.MaxLanes[AnchorTop])
	}
	if p.Layout.MaxLanes[AnchorBottom] < 2 {
		t.Fatalf("bottom lanes = %d, want >= 2", p.Layout.MaxLanes[AnchorBottom])
	}
}

func TestSplitDoesNotMutateInputIDs(t *testing.T) {
	cues := []Cue{{Text: "Anonymous line.", Start: 0, End: 1000}}
	_ = Split(cues)
	if cues[0].ID != "" {
		t.Fatalf("Split mutated input cue ID to %q", cues[0].ID)
	}
}

func TestEveryEntryHasLaneAndGroupState(t *testing.T) {
	p := Split(loadFixtureCues(t))
	for _, e := range p.Entries {
		if e.Lane < 0 {
			t.Fatalf("entry %s has no lane", e.ID)
		}
		if e.Start > e.End {
			t.Fatalf("entry %s bad interval", e.ID)
		}
	}
}

func TestWrappedDialogueMergedIntoOneEntry(t *testing.T) {
	cues := []Cue{{
		ID: "w", Index: 1, Start: 0, End: 2000,
		Text: "Hello there,\nhow are you?",
	}}
	p := Split(cues)
	dialogue := p.LayerEntries(RoleDialogue)
	if len(dialogue) != 1 {
		t.Fatalf("expected one merged dialogue entry, got %d: %+v", len(dialogue), dialogue)
	}
	if dialogue[0].Text != "Hello there,\nhow are you?" {
		t.Fatalf("merged text = %q", dialogue[0].Text)
	}
	if dialogue[0].Lane != 0 || len(p.Layout.Groups) != 0 {
		t.Fatalf("wrapped block should not overlap-stack: lane=%d groups=%d", dialogue[0].Lane, len(p.Layout.Groups))
	}
}

func TestDifferentSpeakersStaySeparate(t *testing.T) {
	cues := []Cue{{
		ID: "d", Index: 1, Start: 0, End: 3000,
		Text: "- MARY: Wait!\n- JOHN: Go.",
	}}
	p := Split(cues)
	dialogue := p.LayerEntries(RoleDialogue)
	if len(dialogue) != 2 {
		t.Fatalf("expected two speaker entries, got %d", len(dialogue))
	}
	if dialogue[0].Lane == dialogue[1].Lane {
		t.Fatalf("simultaneous different speakers need distinct lanes: %d", dialogue[0].Lane)
	}
}
