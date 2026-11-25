package opusalign

import "testing"

func TestSyncCueByPacketRefDoesNotBackfill(t *testing.T) {
	report, err := BuildTimeline(TimelineInput{
		PreSkip: 120,
		Packets: []Packet{
			{ID: "p0", TOC: bytePtr(0x00), Text: "开头"},
			{ID: "p1", TOC: bytePtr(0x00), Received: boolPtr(false), GeneratorText: "原文"},
		},
		Cues: []Cue{{ID: "c1", PacketID: "p1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := report.Subtitles[0]
	if got.Status != IssueMissing || got.Text != "" || got.TextSource != TextLost {
		t.Fatalf("synced cue = %+v", got)
	}
	if got.Range != (SampleRange{360, 840}) {
		t.Fatalf("lost cue true range = %+v", got.Range)
	}
}

func TestAbsoluteCueMeasuresMissingOverlap(t *testing.T) {
	report, err := BuildTimeline(TimelineInput{
		Packets: []Packet{
			{TOC: bytePtr(0x00), GranulePosition: int64Ptr(480)},
			{TOC: bytePtr(0x00), GranulePosition: int64Ptr(1440)},
		},
		Cues: []Cue{{ID: "c", StartSample: 400, EndSample: 1100, Text: "跨缺口"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := report.Subtitles[0]
	if got.Status != "partial_missing" {
		t.Fatalf("status = %q (%+v)", got.Status, got)
	}
	if got.MissingSamples != 480 {
		t.Fatalf("missing overlap = %d, want 480", got.MissingSamples)
	}
}

func TestFormatSRTTime(t *testing.T) {
	tests := []struct {
		sample int64
		want   string
	}{
		{0, "00:00:00,000"},
		{48, "00:00:00,001"},
		{960, "00:00:00,020"},
		{48000 * 61, "00:01:01,000"},
	}
	for _, tt := range tests {
		if got := FormatSRTTime(tt.sample); got != tt.want {
			t.Fatalf("FormatSRTTime(%d) = %q, want %q", tt.sample, got, tt.want)
		}
	}
}

func TestEndOfStreamTrimIsNotOverlap(t *testing.T) {
	report, err := BuildTimeline(TimelineInput{
		Packets: []Packet{
			{TOC: bytePtr(0x00), GranulePosition: int64Ptr(480)},
			{TOC: bytePtr(0x00), GranulePosition: int64Ptr(900), EndOfStream: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	last := report.Intervals[1]
	if last.EndTrimSamples != 60 {
		t.Fatalf("trim = %d, interval=%+v", last.EndTrimSamples, last)
	}
	for _, issue := range last.Issues {
		if issue.Code == IssueOverlap || issue.Code == IssueDuplicate {
			t.Fatalf("end trim misclassified: %+v", last.Issues)
		}
	}
	if last.Playback != (SampleRange{480, 900}) {
		t.Fatalf("trimmed range = %+v", last.Playback)
	}
}
