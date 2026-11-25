package scheduler

import (
	"strings"
	"testing"
)

func TestLongVersionAvoidsDialogueSoundAndSubtitles(t *testing.T) {
	plan := Plan{
		Timeline: Timeline{
			DurationMS: 30_000,
			Dialogue: []Interval{{
				ID: "d1", StartMS: 5_000, EndMS: 7_000, Text: "对白一",
			}},
			SoundEffects: []Interval{{
				ID: "fx1", StartMS: 10_000, EndMS: 12_000, Text: "关门声",
			}},
			Subtitles: []Interval{{
				ID: "sub1", StartMS: 15_000, EndMS: 16_000, Text: "字幕一",
			}},
		},
		Cues: []Cue{{
			ID: "ad1", AnchorMS: 10_000,
			Window: &Window{StartMS: 0, EndMS: 20_000},
			Long:   &Variant{DurationMS: 4_000},
			Short:  &Variant{DurationMS: 2_000},
		}},
	}

	result, err := Schedule(plan)
	if err != nil {
		t.Fatalf("Schedule() error = %v", err)
	}
	if len(result.Placements) != 1 {
		t.Fatalf("placements = %d, want 1; omitted = %+v", len(result.Placements), result.Omitted)
	}
	got := result.Placements[0]
	if got.Variant != "long" || got.StartMS != 1_000 || got.EndMS != 5_000 {
		t.Fatalf("placement = %+v, want long at 1000-5000", got)
	}
	if len(got.Conflicts) != 0 {
		t.Fatalf("long placement conflicts = %+v, want none", got.Conflicts)
	}
	if result.Summary.Constraints != 4 {
		t.Fatalf("constraints = %d, want 4 (3 fixed + scheduled AD)", result.Summary.Constraints)
	}
	if result.Summary.ConstraintCounts[string(ConstraintAudioDesc)] != 1 {
		t.Fatalf("audio-description constraints = %+v", result.Summary.ConstraintCounts)
	}
}

func TestShortVersionReportsDroppedSegmentsAndConflicts(t *testing.T) {
	plan := Plan{
		Timeline: Timeline{
			Dialogue: []Interval{{
				ID: "d1", StartMS: 0, EndMS: 3_500, Text: "人物对白",
			}},
			SoundEffects: []Interval{{
				ID: "fx1", StartMS: 6_500, EndMS: 10_000, Text: "音乐渐强",
			}},
		},
		Cues: []Cue{{
			ID: "ad1", AnchorMS: 5_000,
			Window: &Window{StartMS: 0, EndMS: 10_000},
			Long: &Variant{Segments: []Segment{
				{ID: "s1", Text: "他推开门", DurationMS: 2_000},
				{ID: "s2", Text: "雨水落在地板上", DurationMS: 2_000},
			}},
			Short: &Variant{Segments: []Segment{
				{ID: "s1", Text: "他推开门", DurationMS: 2_000},
			}},
		}},
	}

	result, err := Schedule(plan)
	if err != nil {
		t.Fatalf("Schedule() error = %v", err)
	}
	if len(result.Placements) != 1 {
		t.Fatalf("placements = %d, want 1; omitted = %+v", len(result.Placements), result.Omitted)
	}
	got := result.Placements[0]
	if got.Variant != "short" || got.StartMS != 4_000 || got.EndMS != 6_000 {
		t.Fatalf("placement = %+v, want short at 4000-6000", got)
	}
	if len(got.DroppedInfo) != 1 || got.DroppedInfo[0].SegmentID != "s2" {
		t.Fatalf("dropped info = %+v, want only segment s2", got.DroppedInfo)
	}
	if len(got.Conflicts) != 2 {
		t.Fatalf("conflicts = %+v, want dialogue and sound effect", got.Conflicts)
	}
	if !strings.Contains(got.Explanation, "舍弃长版片段") ||
		!strings.Contains(got.Explanation, "雨水落在地板上") {
		t.Fatalf("explanation does not identify dropped information: %q", got.Explanation)
	}
}

func TestOmissionExplainsAllDroppedInformation(t *testing.T) {
	plan := Plan{
		Timeline: Timeline{
			SoundEffects: []Interval{{
				ID: "fx1", StartMS: 0, EndMS: 5_000, Text: "爆炸声",
			}},
		},
		Cues: []Cue{{
			ID: "ad1", AnchorMS: 2_500,
			Window: &Window{StartMS: 0, EndMS: 5_000},
			Long: &Variant{Segments: []Segment{
				{ID: "s1", Text: "玻璃碎裂", DurationMS: 2_000},
				{ID: "s2", Text: "人群散开", DurationMS: 2_000},
			}},
			Short: &Variant{DurationMS: 2_000},
		}},
	}

	result, err := Schedule(plan)
	if err != nil {
		t.Fatalf("Schedule() error = %v", err)
	}
	if len(result.Placements) != 0 || len(result.Omitted) != 1 {
		t.Fatalf("placements=%d omitted=%d, want 0 and 1", len(result.Placements), len(result.Omitted))
	}
	got := result.Omitted[0]
	if got.Reason != "no_conflict_free_slot" {
		t.Fatalf("reason = %q", got.Reason)
	}
	if len(got.DroppedInfo) != 2 || len(got.Conflicts) != 1 {
		t.Fatalf("omission = %+v, want two dropped segments and one conflict", got)
	}
	if !strings.Contains(got.Explanation, "整条描述已舍弃") ||
		!strings.Contains(got.Explanation, "爆炸声") {
		t.Fatalf("explanation is incomplete: %q", got.Explanation)
	}
}
