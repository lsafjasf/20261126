package audioedit

import (
	"strings"
	"testing"
)

func i64(value int64) *int64 { return &value }

func TestAACCodecDelayIsPrerollNotLost(t *testing.T) {
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
		Clips: []Clip{{ID: "c1", SourceID: "aac"}},
	}

	table, err := Compile(project)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	seg := table.Segments[0]
	if seg.TimelineStart != 0 || seg.TimelineEnd != 48000 || seg.OutputSamples != 48000 {
		t.Fatalf("unexpected timeline: %+v", seg)
	}
	if seg.DecodeStart != 0 || seg.ContentDecodeStart != 2112 {
		t.Fatalf("delay was not mapped into decode coordinates: %+v", seg)
	}
	if seg.PrerollSamples != 2112 {
		t.Fatalf("preroll = %d, want 2112", seg.PrerollSamples)
	}
	if seg.DecodeEnd != 50176 || seg.TailTrimSamples != 64 {
		t.Fatalf("tail trim = %d (decode end %d), want 64 (50176)",
			seg.TailTrimSamples, seg.DecodeEnd)
	}
}

func TestOpusPreSkipAndFrameAlignedSlice(t *testing.T) {
	project := Project{
		Params: Params{SampleRate: 48000},
		Sources: []Source{{
			ID:         "opus",
			Format:     FormatOpus,
			SampleRate: 48000,
			Edit:       Edit{MediaEnd: 10000},
		}},
		Clips: []Clip{{
			ID:          "c1",
			SourceID:    "opus",
			SourceStart: 100,
			SourceEnd:   2020,
		}},
	}

	table, err := Compile(project)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	seg := table.Segments[0]
	if seg.CodecDelay != 312 || seg.ContentDecodeStart != 412 || seg.DecodeStart != 0 {
		t.Fatalf("unexpected decode mapping: %+v", seg)
	}
	if seg.PrerollSamples != 412 {
		t.Fatalf("preroll = %d, want 412", seg.PrerollSamples)
	}
	if seg.ContentDecodeEnd != 2332 || seg.DecodeEnd != 2880 || seg.TailTrimSamples != 548 {
		t.Fatalf("unexpected tail mapping: %+v", seg)
	}
	if seg.OutputSamples != 1920 || seg.TimelineEnd != 1920 {
		t.Fatalf("unexpected output mapping: %+v", seg)
	}
}

func TestAdjacentSourcesDoNotAccumulateDrop(t *testing.T) {
	project := Project{
		Params: Params{SampleRate: 48000},
		Sources: []Source{
			{ID: "a", Format: FormatAAC, SampleRate: 48000, Edit: Edit{MediaEnd: 4000}},
			{ID: "b", Format: FormatOpus, SampleRate: 48000, Edit: Edit{MediaEnd: 4000}},
		},
		Clips: []Clip{
			{ID: "c1", SourceID: "a", SourceEnd: 2000},
			{ID: "c2", SourceID: "b", SourceEnd: 3000},
		},
	}

	table, err := Compile(project)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if got := table.Segments[0].TimelineEnd; got != 2000 {
		t.Fatalf("first clip ends at %d, want 2000", got)
	}
	if got := table.Segments[1].TimelineStart; got != 2000 {
		t.Fatalf("second clip starts at %d, want 2000", got)
	}
	if table.TotalOutputSamples != 5000 {
		t.Fatalf("total output = %d, want 5000", table.TotalOutputSamples)
	}
	if table.Segments[0].PrerollSamples != 1024 ||
		table.Segments[1].PrerollSamples != 312 {
		t.Fatalf("unexpected per-source preroll: %+v", table.Segments)
	}
}

func TestClipAtEditTailConsumesDeclaredTrailer(t *testing.T) {
	project := Project{
		Params: Params{SampleRate: 48000},
		Sources: []Source{{
			ID:             "opus",
			Format:         FormatOpus,
			SampleRate:     48000,
			EncodedSamples: i64(12480),
			Edit:           Edit{MediaEnd: 10000},
		}},
		Clips: []Clip{{ID: "tail", SourceID: "opus"}},
	}

	table, err := Compile(project)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	seg := table.Segments[0]
	if seg.DecodeEnd != 12480 {
		t.Fatalf("decode end = %d, want 12480", seg.DecodeEnd)
	}
	if seg.TailTrimSamples != 2168 {
		t.Fatalf("tail trim = %d, want 2168", seg.TailTrimSamples)
	}
	if seg.OutputSamples != 10000 {
		t.Fatalf("output = %d, want 10000", seg.OutputSamples)
	}
}

func TestCrossRateTimeline(t *testing.T) {
	project := Project{
		Params: Params{SampleRate: 48000},
		Sources: []Source{{
			ID:         "aac44",
			Format:     FormatAAC,
			SampleRate: 44100,
			Edit:       Edit{MediaEnd: 44100},
		}},
		Clips: []Clip{{ID: "c1", SourceID: "aac44", SourceEnd: 44100}},
	}

	table, err := Compile(project)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if table.Segments[0].OutputSamples != 48000 {
		t.Fatalf("output = %d, want 48000", table.Segments[0].OutputSamples)
	}
}

func TestDefaultCodecSpecs(t *testing.T) {
	aac, ok := Spec(FormatAAC)
	if !ok || aac.Delay != 1024 || aac.FrameSamples != 1024 {
		t.Fatalf("unexpected AAC spec: %+v", aac)
	}
	opus, ok := Spec(FormatOpus)
	if !ok || opus.Delay != 312 || opus.FrameSamples != 960 || opus.NativeRate != 48000 {
		t.Fatalf("unexpected Opus spec: %+v", opus)
	}
}

func TestInvalidSourceRangeIsRejected(t *testing.T) {
	project := Project{
		Params: Params{SampleRate: 48000},
		Sources: []Source{{
			ID: "a", Format: FormatAAC, SampleRate: 48000,
			Edit: Edit{MediaEnd: 1000},
		}},
		Clips: []Clip{{ID: "bad", SourceID: "a", SourceStart: 900, SourceEnd: 1200}},
	}
	_, err := Compile(project)
	if err == nil || !strings.Contains(err.Error(), "edit.media_end") {
		t.Fatalf("error = %v, want edit.media_end range error", err)
	}
}

func TestOpusRequiresNativeRate(t *testing.T) {
	project := Project{
		Params: Params{SampleRate: 48000},
		Sources: []Source{{
			ID: "opus", Format: FormatOpus, SampleRate: 44100,
			Edit: Edit{MediaEnd: 1000},
		}},
		Clips: []Clip{{ID: "c", SourceID: "opus"}},
	}
	if _, err := Compile(project); err == nil {
		t.Fatal("expected Opus sample-rate error")
	}
}
