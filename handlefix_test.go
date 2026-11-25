package handlefix

import (
	"errors"
	"testing"
)

func testProject() Project {
	return Project{
		SequenceBounds: Range{Start: 0, End: 30},
		Clips: []Clip{
			{ID: "a", Media: Range{0, 24}, Source: Range{6, 16}, Sequence: Range{0, 10}},
			{ID: "b", Media: Range{0, 30}, Source: Range{10, 20}, Sequence: Range{10, 20}},
			{ID: "c", Media: Range{0, 20}, Source: Range{4, 14}, Sequence: Range{20, 30}},
		},
		Transitions: []Transition{
			{ID: "t1", LeftClipID: "a", RightClipID: "b", Cut: 10, Head: 4, Tail: 4},
			{ID: "t2", LeftClipID: "b", RightClipID: "c", Cut: 20, Head: 3, Tail: 6},
		},
	}
}

// oldTrimHandlesBug reproduces the faulty export step: positive handle
// lengths were passed as inward trims, moving both transition-facing edges
// into the reserved source material.
func oldTrimHandlesBug(t *testing.T, project Project) []EdgeEdit {
	t.Helper()
	edits := make([]EdgeEdit, 0)
	for _, tr := range project.Transitions {
		left := project.Clip(tr.LeftClipID)
		right := project.Clip(tr.RightClipID)

		before := left.Source.End
		trimmed, err := trimEndInward(left.Source, tr.Tail)
		if err != nil {
			t.Fatalf("bad left trim setup: %v", err)
		}
		left.Source = trimmed
		edits = append(edits, EdgeEdit{
			Step: StepHandleTrim, ClipID: left.ID, Edge: EdgeSourceEnd,
			Before: before, After: left.Source.End,
		})

		before = right.Source.Start
		trimmedStart, err := trimStartInward(right.Source, tr.Head)
		if err != nil {
			t.Fatalf("bad right trim setup: %v", err)
		}
		right.Source = trimmedStart
		edits = append(edits, EdgeEdit{
			Step: StepHandleTrim, ClipID: right.ID, Edge: EdgeSourceStart,
			Before: before, After: right.Source.Start,
		})
	}
	return edits
}

func TestAuditTraceLocatesEveryTransitionSide(t *testing.T) {
	original := testProject()
	damaged := original.Clone()
	directEdits := oldTrimHandlesBug(t, damaged)
	diffEdits := DiffSourceEdges(StepHandleTrim, original, damaged)
	if len(diffEdits) != 4 {
		t.Fatalf("DiffSourceEdges edits = %d, want 4 distinct edges: %+v", len(diffEdits), diffEdits)
	}

	report, err := AuditTrace(damaged, directEdits)
	if err != nil {
		t.Fatal(err)
	}
	if report.Total != 2 {
		t.Fatalf("Total = %d, want 2", report.Total)
	}
	if report.DamagedCount != 2 {
		t.Fatalf("DamagedCount = %d, want both transitions damaged", report.DamagedCount)
	}
	if report.HardCutCount != 2 {
		t.Fatalf("HardCutCount = %d, want unsafe mapping to force hard cuts", report.HardCutCount)
	}
	if len(report.Locations) != 4 {
		t.Fatalf("Locations = %d, want four sides: %+v", len(report.Locations), report.Locations)
	}

	want := map[string]int64{
		"t1:a:outgoing_tail": 4,
		"t1:b:incoming_head": 4,
		"t2:b:outgoing_tail": 6,
		"t2:c:incoming_head": 3,
	}
	for _, loc := range report.Locations {
		key := loc.TransitionID + ":" + loc.ClipID + ":" + loc.Side
		if loc.Step != StepHandleTrim {
			t.Fatalf("%s step = %q, want %q", key, loc.Step, StepHandleTrim)
		}
		if want[key] != loc.LostFrames {
			t.Errorf("%s lost = %d, want %d", key, loc.LostFrames, want[key])
		}
		delete(want, key)
	}
	if len(want) != 0 {
		t.Fatalf("missing damage locations: %+v", want)
	}
}

func TestReverseEditsRestoresExactProject(t *testing.T) {
	original := testProject()
	damaged := original.Clone()
	edits := oldTrimHandlesBug(t, damaged)

	restored, err := ReverseEdits(damaged, edits)
	if err != nil {
		t.Fatal(err)
	}
	for i := range original.Clips {
		if restored.Clips[i].Source != original.Clips[i].Source {
			t.Fatalf("clip %s source = %+v, want %+v",
				original.Clips[i].ID, restored.Clips[i].Source, original.Clips[i].Source)
		}
	}
}

func TestReverseEditsRejectsOutOfBounds(t *testing.T) {
	project := testProject()
	_, err := ReverseEdits(project, []EdgeEdit{{
		Step: "bad", ClipID: "a", Edge: EdgeSourceStart, Before: -1, After: 6,
	}})
	if !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("err = %v, want ErrOutOfBounds", err)
	}
}
