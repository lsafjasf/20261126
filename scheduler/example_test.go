package scheduler_test

import (
	"encoding/json"
	"os"
	"testing"

	"audiodesc/scheduler"
)

func TestExamplePlan(t *testing.T) {
	data, err := os.ReadFile("../examples/plan.json")
	if err != nil {
		t.Fatal(err)
	}
	var plan scheduler.Plan
	if err := json.Unmarshal(data, &plan); err != nil {
		t.Fatalf("decode example: %v", err)
	}
	result, err := scheduler.Schedule(plan)
	if err != nil {
		t.Fatalf("Schedule() error = %v", err)
	}
	if got, want := len(result.Placements), 2; got != want {
		t.Fatalf("placements = %d, want %d", got, want)
	}
	if got, want := len(result.Omitted), 1; got != want {
		t.Fatalf("omitted = %d, want %d", got, want)
	}
	if result.Summary.LongPlaced != 1 || result.Summary.ShortPlaced != 1 {
		t.Fatalf("summary = %+v, want one long and one short", result.Summary)
	}
	for _, placement := range result.Placements {
		for _, constraint := range result.Constraints {
			if constraint.CueID == placement.CueID {
				continue
			}
			if placement.StartMS < constraint.EndMS && constraint.StartMS < placement.EndMS {
				t.Fatalf("placement %+v overlaps constraint %+v", placement, constraint)
			}
		}
	}
}
