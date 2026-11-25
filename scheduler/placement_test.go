package scheduler

import (
	"strings"
	"testing"
)

func TestScheduledDescriptionBecomesConstraint(t *testing.T) {
	plan := Plan{
		Cues: []Cue{
			{
				ID: "high", AnchorMS: 5_000, Priority: 10,
				Window: &Window{StartMS: 0, EndMS: 10_000},
				Long:   &Variant{DurationMS: 4_000},
			},
			{
				ID: "low", AnchorMS: 5_000, Priority: 1,
				Window: &Window{StartMS: 0, EndMS: 10_000},
				Long:   &Variant{DurationMS: 4_000},
				Short:  &Variant{DurationMS: 2_000},
			},
		},
	}

	result, err := Schedule(plan)
	if err != nil {
		t.Fatalf("Schedule() error = %v", err)
	}
	if len(result.Placements) != 2 {
		t.Fatalf("placements = %+v, omitted = %+v", result.Placements, result.Omitted)
	}
	byID := map[string]Placement{}
	for _, placement := range result.Placements {
		byID[placement.CueID] = placement
	}
	if got := byID["high"]; got.Variant != "long" || got.StartMS != 3_000 || got.EndMS != 7_000 {
		t.Fatalf("high-priority placement = %+v", got)
	}
	low := byID["low"]
	if low.Variant != "short" || low.StartMS != 7_000 || low.EndMS != 9_000 {
		t.Fatalf("low-priority placement = %+v, want short at 7000-9000", low)
	}
	if len(low.Conflicts) != 1 || low.Conflicts[0].Kind != ConstraintAudioDesc {
		t.Fatalf("low conflicts = %+v, want scheduled audio description", low.Conflicts)
	}
}

func TestLongVersionWinsEvenWhenShortCanSitAtAnchor(t *testing.T) {
	plan := Plan{
		Timeline: Timeline{
			Dialogue: []Interval{{
				ID: "d1", StartMS: 8_000, EndMS: 12_000, Text: "对白",
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
		t.Fatalf("placements = %+v", result.Placements)
	}
	got := result.Placements[0]
	if got.Variant != "long" || got.StartMS != 12_000 || got.EndMS != 16_000 {
		t.Fatalf("placement = %+v, want long at 12000-16000", got)
	}
}

func TestClearanceIsAppliedAroundEveryConstraint(t *testing.T) {
	plan := Plan{
		Timeline: Timeline{
			Dialogue: []Interval{{
				ID: "d1", StartMS: 0, EndMS: 1_000,
			}},
			SoundEffects: []Interval{{
				ID: "fx1", StartMS: 5_000, EndMS: 10_000,
			}},
		},
		Cues: []Cue{{
			ID: "ad1", AnchorMS: 3_000,
			Window: &Window{StartMS: 0, EndMS: 10_000},
			Long:   &Variant{DurationMS: 3_000},
		}},
		Options: Options{MinClearanceMS: 500},
	}

	result, err := Schedule(plan)
	if err != nil {
		t.Fatalf("Schedule() error = %v", err)
	}
	if len(result.Placements) != 1 {
		t.Fatalf("placements = %+v, omitted = %+v", result.Placements, result.Omitted)
	}
	got := result.Placements[0]
	if got.StartMS != 1_500 || got.EndMS != 4_500 {
		t.Fatalf("placement = %+v, want exact clearance gap 1500-4500", got)
	}
}

func TestDefaultWindowIsClampedToMediaDuration(t *testing.T) {
	plan := Plan{
		Timeline: Timeline{DurationMS: 10_000},
		Cues: []Cue{{
			ID: "ad1", AnchorMS: 10_000,
			Long: &Variant{DurationMS: 2_000},
		}},
	}

	result, err := Schedule(plan)
	if err != nil {
		t.Fatalf("Schedule() error = %v", err)
	}
	if len(result.Placements) != 1 {
		t.Fatalf("placements = %+v, omitted = %+v", result.Placements, result.Omitted)
	}
	got := result.Placements[0]
	if got.StartMS != 8_000 || got.EndMS != 10_000 {
		t.Fatalf("placement = %+v, want 8000-10000", got)
	}
}

func TestInputValidation(t *testing.T) {
	tests := []struct {
		name string
		plan Plan
		want string
	}{
		{
			name: "duplicate cue id",
			plan: Plan{Cues: []Cue{
				{ID: "same", Long: &Variant{DurationMS: 1_000}},
				{ID: "same", Long: &Variant{DurationMS: 1_000}},
			}},
			want: "duplicate cue id",
		},
		{
			name: "missing variants",
			plan: Plan{Cues: []Cue{{ID: "ad1"}}},
			want: "at least one",
		},
		{
			name: "invalid constraint",
			plan: Plan{Timeline: Timeline{Dialogue: []Interval{{
				ID: "d1", StartMS: 5_000, EndMS: 5_000,
			}}}},
			want: "end_ms must be greater",
		},
		{
			name: "window outside timeline",
			plan: Plan{
				Timeline: Timeline{DurationMS: 5_000},
				Cues: []Cue{{
					ID: "ad1", AnchorMS: 2_000,
					Window: &Window{StartMS: 0, EndMS: 8_000},
					Long:   &Variant{DurationMS: 1_000},
				}},
			},
			want: "exceeds timeline duration",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Schedule(test.plan)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Schedule() error = %v, want containing %q", err, test.want)
			}
		})
	}
}
