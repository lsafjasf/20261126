package audioedit

import "testing"

func editorFixture(t *testing.T) *Editor {
	t.Helper()
	project := Project{
		Params: Params{SampleRate: 48000},
		Sources: []Source{{
			ID:             "aac",
			Format:         FormatAAC,
			SampleRate:     48000,
			CodecDelay:     i64(2112),
			EncodedSamples: i64(50176),
			Edit:           Edit{MediaEnd: 48000},
		}},
		Clips: []Clip{
			{ID: "one", SourceID: "aac", SourceEnd: 2000},
			{ID: "two", SourceID: "aac", SourceStart: 2000, SourceEnd: 5000},
		},
	}
	editor, err := NewEditor(project)
	if err != nil {
		t.Fatalf("NewEditor: %v", err)
	}
	return editor
}

func TestChangingDelayRecomputesEverySegment(t *testing.T) {
	editor := editorFixture(t)
	before := editor.Table()
	if err := editor.SetCodecDelay("aac", 1024); err != nil {
		t.Fatalf("SetCodecDelay: %v", err)
	}
	after := editor.Table()

	if after.Revision != before.Revision+1 {
		t.Fatalf("revision = %d, want %d", after.Revision, before.Revision+1)
	}
	if after.TotalPrerollSamples >= before.TotalPrerollSamples {
		t.Fatalf("preroll did not shrink: before=%d after=%d",
			before.TotalPrerollSamples, after.TotalPrerollSamples)
	}
	for i := range after.Segments {
		if after.Segments[i].CodecDelay != 1024 {
			t.Fatalf("segment %d kept old delay: %+v", i, after.Segments[i])
		}
	}
	if after.Segments[1].ContentDecodeStart != 3024 {
		t.Fatalf("second segment content start = %d, want 3024",
			after.Segments[1].ContentDecodeStart)
	}
}

func TestGapChangeShiftsAllFollowingSegments(t *testing.T) {
	editor := editorFixture(t)
	before := editor.Table()
	if err := editor.SetGapSamples(100); err != nil {
		t.Fatalf("SetGapSamples: %v", err)
	}
	after := editor.Table()

	if after.Segments[0].TimelineStart != 0 {
		t.Fatalf("first segment moved: %+v", after.Segments[0])
	}
	want := before.Segments[1].TimelineStart + 100
	if after.Segments[1].TimelineStart != want {
		t.Fatalf("second start = %d, want %d", after.Segments[1].TimelineStart, want)
	}
	if after.TotalOutputSamples != before.TotalOutputSamples+100 {
		t.Fatalf("total = %d, want %d",
			after.TotalOutputSamples, before.TotalOutputSamples+100)
	}
}

func TestFailedMutationKeepsPreviousTable(t *testing.T) {
	editor := editorFixture(t)
	before := editor.Table()
	if err := editor.SetGapSamples(-1); err == nil {
		t.Fatal("expected validation error")
	}
	after := editor.Table()
	if after.Revision != before.Revision {
		t.Fatalf("revision changed after failed update: %d -> %d",
			before.Revision, after.Revision)
	}
	if after.TotalOutputSamples != before.TotalOutputSamples ||
		after.GapSamples != before.GapSamples {
		t.Fatalf("table changed after failed update: before=%+v after=%+v", before, after)
	}
}

func TestSampleRateChangeRescalesTimeline(t *testing.T) {
	editor := editorFixture(t)
	if err := editor.SetProjectSampleRate(24000); err != nil {
		t.Fatalf("SetProjectSampleRate: %v", err)
	}
	table := editor.Table()
	if table.SampleRate != 24000 {
		t.Fatalf("sample rate = %d", table.SampleRate)
	}
	if table.Segments[0].OutputSamples != 1000 ||
		table.Segments[1].OutputSamples != 1500 {
		t.Fatalf("unexpected rescaled segments: %+v", table.Segments)
	}
	if table.Segments[1].TimelineStart != 1000 {
		t.Fatalf("second start = %d, want 1000", table.Segments[1].TimelineStart)
	}
}

func TestReturnedTableIsIndependent(t *testing.T) {
	editor := editorFixture(t)
	table := editor.Table()
	table.Segments[0].TimelineEnd = 999999
	again := editor.Table()
	if again.Segments[0].TimelineEnd == 999999 {
		t.Fatal("Editor returned an aliased table")
	}
}
