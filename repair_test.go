package handlefix

import (
	"errors"
	"reflect"
	"testing"
)

func TestPrepareExportExtendsHandlesAndIsIdempotent(t *testing.T) {
	project := testProject()
	exported, report, err := PrepareExport(project)
	if err != nil {
		t.Fatal(err)
	}
	if report.HardCutCount != 0 || report.DamagedCount != 0 {
		t.Fatalf("report = %+v, want healthy transitions", report)
	}

	want := map[string]Clip{
		"a": {ID: "a", Media: Range{0, 24}, Source: Range{6, 20}, Sequence: Range{0, 14}},
		"b": {ID: "b", Media: Range{0, 30}, Source: Range{6, 26}, Sequence: Range{6, 26}},
		"c": {ID: "c", Media: Range{0, 20}, Source: Range{1, 14}, Sequence: Range{17, 30}},
	}
	for _, clip := range exported.Clips {
		expected := want[clip.ID]
		if clip.Source != expected.Source || clip.Sequence != expected.Sequence {
			t.Errorf("clip %s = source%+v seq%+v, want source%+v seq%+v",
				clip.ID, clip.Source, clip.Sequence, expected.Source, expected.Sequence)
		}
		if !clip.Source.within(clip.Media) {
			t.Errorf("clip %s source %+v outside media %+v", clip.ID, clip.Source, clip.Media)
		}
		if !clip.Sequence.within(exported.SequenceBounds) {
			t.Errorf("clip %s sequence %+v outside bounds %+v", clip.ID, clip.Sequence, exported.SequenceBounds)
		}
	}

	again, secondReport, err := PrepareExport(exported)
	if err != nil {
		t.Fatal(err)
	}
	if secondReport.HardCutCount != 0 {
		t.Fatalf("re-export report has hard cuts: %+v", secondReport)
	}
	if !reflect.DeepEqual(again.Clips, exported.Clips) {
		t.Fatalf("re-export changed ranges:\nfirst=%+v\nsecond=%+v", exported.Clips, again.Clips)
	}
	if !reflect.DeepEqual(project, testProject()) {
		t.Fatal("PrepareExport mutated the input project")
	}
}

func TestPrepareExportClampsToMediaAndSequenceBounds(t *testing.T) {
	project := Project{
		SequenceBounds: Range{Start: 0, End: 6},
		Clips: []Clip{
			{ID: "left", Media: Range{Start: 0, End: 7}, Source: Range{Start: 2, End: 5}, Sequence: Range{0, 3}},
			{ID: "right", Media: Range{Start: 2, End: 10}, Source: Range{Start: 3, End: 6}, Sequence: Range{3, 6}},
		},
		Transitions: []Transition{{
			ID: "tr", LeftClipID: "left", RightClipID: "right", Cut: 3, Head: 4, Tail: 4,
		}},
	}

	exported, report, err := PrepareExport(project)
	if err != nil {
		t.Fatal(err)
	}
	tr := report.Transitions[0]
	if tr.EffectiveTail != 2 || tr.EffectiveHead != 1 {
		t.Fatalf("effective head/tail = %d/%d, want 1/2", tr.EffectiveHead, tr.EffectiveTail)
	}
	if tr.HardCut {
		t.Fatalf("partial handles should remain a transition: %+v", tr)
	}

	left := exported.Clip("left")
	right := exported.Clip("right")
	if left.Source != (Range{2, 7}) || left.Sequence != (Range{0, 5}) {
		t.Errorf("left = source%+v seq%+v, want source[2,7) seq[0,5)", left.Source, left.Sequence)
	}
	if right.Source != (Range{2, 6}) || right.Sequence != (Range{2, 6}) {
		t.Errorf("right = source%+v seq%+v, want source[2,6) seq[2,6)", right.Source, right.Sequence)
	}
}

func TestPrepareExportZeroAvailableHandleIsSafeHardCut(t *testing.T) {
	project := Project{
		SequenceBounds: Range{0, 10},
		Clips: []Clip{
			{ID: "left", Media: Range{0, 5}, Source: Range{0, 5}, Sequence: Range{0, 5}},
			{ID: "right", Media: Range{5, 10}, Source: Range{5, 10}, Sequence: Range{5, 10}},
		},
		Transitions: []Transition{SymmetricTransition("tr", "left", "right", 5, 4)},
	}

	exported, report, err := PrepareExport(project)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Transitions[0].HardCut {
		t.Fatalf("expected bounded hard cut report: %+v", report.Transitions[0])
	}
	if !reflect.DeepEqual(exported.Clips, project.Clips) {
		t.Fatalf("zero handles changed clips: %+v", exported.Clips)
	}
}

func TestPrepareExportRejectsBrokenMapping(t *testing.T) {
	project := testProject()
	project.Clips[0].Source.End -= 4
	_, _, err := PrepareExport(project)
	if !errors.Is(err, errMapping) {
		t.Fatalf("err = %v, want errMapping", err)
	}
}
