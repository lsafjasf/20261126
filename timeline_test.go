package opusalign

import (
	"encoding/json"
	"strings"
	"testing"
)

func int64Ptr(v int64) *int64 { return &v }
func boolPtr(v bool) *bool    { return &v }
func bytePtr(v byte) *byte    { return &v }

func TestPreSkipShiftsTrueInterval(t *testing.T) {
	report, err := BuildTimeline(TimelineInput{
		PreSkip: 120,
		Packets: []Packet{
			{TOC: bytePtr(0x00)},
			{TOC: bytePtr(0x00)},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	first := report.Intervals[0]
	if first.Raw != (SampleRange{0, 480}) {
		t.Fatalf("raw first = %+v", first.Raw)
	}
	if first.Playback != (SampleRange{-120, 360}) {
		t.Fatalf("playback first = %+v", first.Playback)
	}
	if first.Audible != (SampleRange{0, 360}) {
		t.Fatalf("audible first = %+v", first.Audible)
	}
	second := report.Intervals[1]
	if second.Playback != (SampleRange{360, 840}) {
		t.Fatalf("second playback = %+v", second.Playback)
	}
}

func TestGranuleRevealsImplicitPaddingGap(t *testing.T) {
	report, err := BuildTimeline(TimelineInput{
		Packets: []Packet{
			{TOC: bytePtr(0x00), GranulePosition: int64Ptr(480)},
			{TOC: bytePtr(0x00), GranulePosition: int64Ptr(1440)},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	second := report.Intervals[1]
	if second.Playback != (SampleRange{960, 1440}) {
		t.Fatalf("second true range = %+v", second.Playback)
	}
	if len(report.MissingRanges) != 1 {
		t.Fatalf("missing ranges = %+v", report.MissingRanges)
	}
	gap := report.MissingRanges[0]
	if gap.Range != (SampleRange{480, 960}) || gap.Samples != 480 || gap.Kind != "implicit_padding" {
		t.Fatalf("gap = %+v", gap)
	}
	if report.Stats.MissingSamples != 480 {
		t.Fatalf("stats = %+v", report.Stats)
	}
}

func TestDuplicateSequenceAndInterval(t *testing.T) {
	report, err := BuildTimeline(TimelineInput{
		Packets: []Packet{
			{Sequence: int64Ptr(7), TOC: bytePtr(0x00), GranulePosition: int64Ptr(480)},
			{Sequence: int64Ptr(7), TOC: bytePtr(0x00), GranulePosition: int64Ptr(480)},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Duplicates) == 0 {
		t.Fatalf("expected duplicate markers: %+v", report)
	}
	if report.Stats.DuplicateCount != len(report.Duplicates) {
		t.Fatalf("stats duplicate count %d != markers %d", report.Stats.DuplicateCount, len(report.Duplicates))
	}
}

func TestIllegalDurationAndTOCMismatch(t *testing.T) {
	report, err := BuildTimeline(TimelineInput{
		Packets: []Packet{
			{DurationSamples: 100},
			{TOC: bytePtr(0x00), DurationSamples: 960},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.InvalidDurations) != 2 {
		t.Fatalf("invalid durations = %+v", report.InvalidDurations)
	}
	if report.Intervals[1].DurationSamples != 480 {
		t.Fatalf("TOC duration should be authoritative, got %d", report.Intervals[1].DurationSamples)
	}
	if !hasIssue(report.Intervals[1].Issues, IssueTOCMismatch) {
		t.Fatalf("expected mismatch issue: %+v", report.Intervals[1].Issues)
	}
}

func TestLostPacketDoesNotUseGeneratorText(t *testing.T) {
	secret := "生成端原文-不得出现在普通播放"
	report, err := BuildTimeline(TimelineInput{
		Packets: []Packet{
			{Sequence: int64Ptr(1), TOC: bytePtr(0x00), Text: "收到的字幕"},
			{Sequence: int64Ptr(2), TOC: bytePtr(0x00), Received: boolPtr(false), GeneratorText: secret},
			{Sequence: int64Ptr(3), TOC: bytePtr(0x00), Text: "下一条"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	lost := report.Intervals[1]
	if lost.Text != "" || lost.Received {
		t.Fatalf("lost interval must have empty playback text: %+v", lost)
	}
	if lost.TextStatus != TextGeneratorSuppressed {
		t.Fatalf("text status = %q", lost.TextStatus)
	}
	if report.Stats.LostSamples != 480 {
		t.Fatalf("lost samples = %d", report.Stats.LostSamples)
	}
	for _, cue := range report.Subtitles {
		if strings.Contains(cue.Text, secret) {
			t.Fatalf("generator transcript leaked into playback cue: %+v", cue)
		}
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), secret) {
		t.Fatal("generator text must not be serialized into ordinary playback report")
	}
}

func TestSequenceGapMarker(t *testing.T) {
	report, err := BuildTimeline(TimelineInput{
		Packets: []Packet{
			{Sequence: int64Ptr(1), TOC: bytePtr(0x00)},
			{Sequence: int64Ptr(4), TOC: bytePtr(0x00)},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.SequenceGaps) != 1 {
		t.Fatalf("sequence gaps = %+v", report.SequenceGaps)
	}
	gap := report.SequenceGaps[0]
	if gap.FromSequence != 2 || gap.ToSequence != 3 {
		t.Fatalf("sequence gap = %+v", gap)
	}
}
