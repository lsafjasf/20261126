package edlframe

import (
	"encoding/json"
	"os"
	"testing"
)

func loadSample(t *testing.T) Document {
	t.Helper()
	data, err := os.ReadFile("examples/sample.json")
	if err != nil {
		t.Fatalf("read sample: %v", err)
	}
	var doc Document
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse sample: %v", err)
	}
	return doc
}

func findEventReport(t *testing.T, report *Report, id string) EventReport {
	t.Helper()
	for _, event := range report.Events {
		if event.EventID == id {
			return event
		}
	}
	t.Fatalf("event report %s not found", id)
	return EventReport{}
}

func findFixedEvent(t *testing.T, doc *Document, id string) Event {
	t.Helper()
	for _, event := range doc.Events {
		if event.EventID == id {
			return event
		}
	}
	t.Fatalf("fixed event %s not found", id)
	return Event{}
}

func TestSampleDetectsReelAndEveryEventConflict(t *testing.T) {
	report, err := Analyze(loadSample(t), UnifyOptions{ExpandFrames: true})
	if err != nil {
		t.Fatal(err)
	}

	if report.Summary.ReelConflictCount != 1 {
		t.Fatalf("reel conflicts = %d, want 1", report.Summary.ReelConflictCount)
	}
	if report.Summary.EventConflictCount != 4 {
		t.Fatalf("event conflicts = %d, want all four VOL_A rows", report.Summary.EventConflictCount)
	}
	if report.Summary.InvalidEventCount != 1 {
		t.Fatalf("invalid events = %d, want 1", report.Summary.InvalidEventCount)
	}
	if report.Summary.ConvertedEventCount != 3 {
		t.Fatalf("convertible events = %d, want 3", report.Summary.ConvertedEventCount)
	}
	if report.Summary.AffectedFrameCount != 125 {
		t.Fatalf("affected frames = %d, want 125", report.Summary.AffectedFrameCount)
	}

	reel := report.Reels[0]
	if !reel.RateConflict || reel.CanonicalRate != "25" {
		t.Fatalf("unexpected reel report: %+v", reel)
	}
	if got := len(reel.ConflictEventIDs); got != 4 {
		t.Fatalf("reel conflict ids = %v, want 4 rows", reel.ConflictEventIDs)
	}

	first := findEventReport(t, report, "001")
	if first.Source.Impact.Canonical.StartFrame != 250 || first.Source.Impact.Canonical.EndFrame != 300 {
		t.Fatalf("event 001 canonical range = %+v", first.Source.Impact.Canonical)
	}
	if frames := first.Source.Impact.AffectedFrames.Frames; len(frames) != 50 || frames[0] != 250 || frames[49] != 299 {
		t.Fatalf("event 001 expanded frames = len %d first/last %d/%d", len(frames), frames[0], frames[len(frames)-1])
	}

	midnight := findEventReport(t, report, "002")
	if midnight.Source.StartFrame != 2073552 || midnight.Source.EndFrame != 2073600 {
		t.Fatalf("midnight declared range = [%d,%d)", midnight.Source.StartFrame, midnight.Source.EndFrame)
	}
	if !midnight.Source.CrossesMidnight || midnight.Source.DurationFrames != 48 {
		t.Fatalf("midnight interval not unwrapped correctly: %+v", midnight.Source)
	}
	if midnight.Source.Impact.Canonical.StartFrame != 2159950 || midnight.Source.Impact.Canonical.EndFrame != 2160000 {
		t.Fatalf("midnight canonical range = %+v", midnight.Source.Impact.Canonical)
	}

	inherited := findEventReport(t, report, "003")
	if inherited.DeclaredRate != "24" || !inherited.RateConflict {
		t.Fatalf("event 003 should inherit event-table 24fps: %+v", inherited)
	}
	if inherited.Source.Impact.Canonical.StartFrame != 90000 || inherited.Source.Impact.Canonical.EndFrame != 90025 {
		t.Fatalf("event 003 canonical range = %+v", inherited.Source.Impact.Canonical)
	}

	negative := findEventReport(t, report, "004")
	if negative.Convertible {
		t.Fatal("negative-duration event must not be marked convertible")
	}
	if negative.Source.DurationFrames != -24 {
		t.Fatalf("negative duration = %d, want -24", negative.Source.DurationFrames)
	}
	if !hasCode(negative.Issues, "negative_duration") {
		t.Fatalf("missing negative_duration issue: %+v", negative.Issues)
	}
}

func TestUnifyProcessesEveryValidEventAndQuarantinesNegativeRow(t *testing.T) {
	fixed, report, err := Unify(loadSample(t), UnifyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.ConvertedEventCount != 3 {
		t.Fatalf("planned conversions = %d, want 3", report.Summary.ConvertedEventCount)
	}

	event001 := findFixedEvent(t, fixed, "001")
	if event001.Rate != "25" || event001.SourceIn != "00:00:10:00" || event001.SourceOut != "00:00:12:00" {
		t.Fatalf("event 001 was not unified: %+v", event001)
	}

	event002 := findFixedEvent(t, fixed, "002")
	if event002.Rate != "25" || !event002.CrossesMidnight {
		t.Fatalf("event 002 lost rate or midnight marker: %+v", event002)
	}
	if event002.SourceIn != "23:59:58:00" || event002.SourceOut != "00:00:00:00" {
		t.Fatalf("event 002 timecodes changed unexpectedly: %+v", event002)
	}

	event003 := findFixedEvent(t, fixed, "003")
	if event003.Rate != "25" {
		t.Fatalf("event 003 should be made explicit after unify: %+v", event003)
	}

	event004 := findFixedEvent(t, fixed, "004")
	if event004.Rate != "24" || event004.SourceIn != "00:00:20:00" || event004.SourceOut != "00:00:19:00" {
		t.Fatalf("illegal event must remain untouched on explicit old base: %+v", event004)
	}

	rechecked, err := Analyze(*fixed, UnifyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if rechecked.Summary.ReelConflictCount != 0 {
		t.Fatalf("reel declarations should be unified: %+v", rechecked.Reels[0])
	}
	if rechecked.Summary.EventConflictCount != 1 || rechecked.Summary.InvalidEventCount != 1 {
		t.Fatalf("only quarantined row should remain, got conflicts=%d invalid=%d", rechecked.Summary.EventConflictCount, rechecked.Summary.InvalidEventCount)
	}
}

func TestParseRateAliasesAndDropFrameTimecode(t *testing.T) {
	ndf2997, err := ParseTimebase("29.97")
	if err != nil {
		t.Fatal(err)
	}
	broadcast, err := ParseTimebase("30000/1001")
	if err != nil {
		t.Fatal(err)
	}
	if !ndf2997.Rate.Equal(broadcast.Rate) {
		t.Fatalf("29.97 alias = %+v, want %+v", ndf2997.Rate, broadcast.Rate)
	}

	df, err := ParseTimebase("29.97DF")
	if err != nil {
		t.Fatal(err)
	}
	tc, err := ParseTimecode("00:10:00;00", df)
	if err != nil {
		t.Fatal(err)
	}
	if tc.Linear != 17982 {
		t.Fatalf("DF 10-minute frame = %d, want 17982", tc.Linear)
	}
	if got := FormatTimecode(17982, df); got != "00:10:00;00" {
		t.Fatalf("format DF frame = %q, want 00:10:00;00", got)
	}
}

func TestMidnightWrapCanBeInferredWithoutMaskingNegativeDuration(t *testing.T) {
	tb, err := ParseTimebase("24")
	if err != nil {
		t.Fatal(err)
	}

	wrapped := analyzeInterval("source", "23:59:58:00", "00:00:00:00", false, tb, tb, false, false)
	if !wrapped.Valid || !wrapped.CrossesMidnight || wrapped.DurationFrames != 48 {
		t.Fatalf("23->00 interval should infer midnight wrap: %+v", wrapped)
	}
	if !hasCode(wrapped.Issues, "midnight_wrap_inferred") {
		t.Fatalf("missing inference issue: %+v", wrapped.Issues)
	}

	negative := analyzeInterval("source", "00:00:20:00", "00:00:19:00", false, tb, tb, false, false)
	if negative.Valid || !hasCode(negative.Issues, "negative_duration") {
		t.Fatalf("same-hour backward interval must remain negative: %+v", negative)
	}
}

func TestInferredEventTableRateAppliesToEventsWithoutExplicitRate(t *testing.T) {
	doc := Document{
		SourceList: []SourceEntry{{SourceID: "S1", ReelID: "V1", Rate: "25"}},
		Events: []Event{
			{EventID: "1", ReelID: "V1", SourceID: "S1", Rate: "24", SourceIn: "00:00:00:00", SourceOut: "00:00:01:00"},
			{EventID: "2", ReelID: "V1", SourceID: "S1", SourceIn: "00:00:01:00", SourceOut: "00:00:02:00"},
		},
	}

	report, err := Analyze(doc, UnifyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Reels[0].RateConflict || report.Reels[0].EventTableRate != "24" {
		t.Fatalf("expected inferred reel conflict: %+v", report.Reels[0])
	}
	for _, id := range []string{"1", "2"} {
		event := findEventReport(t, report, id)
		if !event.RateConflict || event.DeclaredRate != "24" {
			t.Fatalf("event %s did not inherit inferred 24fps: %+v", id, event)
		}
	}
}

func TestExplicitNegativeDurationFieldIsInvalid(t *testing.T) {
	negativeDuration := int64(-7)
	doc := Document{
		Reels: []ReelDecl{{ReelID: "V1", SourceListRate: "25", EventTableRate: "24"}},
		Events: []Event{{
			EventID: "bad", ReelID: "V1", Rate: "24",
			SourceIn: "00:00:01:00", SourceOut: "00:00:01:00",
			DurationFrames: &negativeDuration,
		}},
	}

	report, err := Analyze(doc, UnifyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	event := findEventReport(t, report, "bad")
	if event.Convertible {
		t.Fatal("explicit negative duration must block conversion")
	}
	if !hasCode(event.Issues, "negative_duration") {
		t.Fatalf("missing negative_duration: %+v", event.Issues)
	}
}

func TestUnifyConvertsExplicitDurationWithoutMutatingInput(t *testing.T) {
	originalDuration := int64(24)
	doc := Document{
		Reels: []ReelDecl{{ReelID: "V1", SourceListRate: "25", EventTableRate: "24"}},
		Events: []Event{{
			EventID: "1", ReelID: "V1", Rate: "24",
			SourceIn: "01:00:00:00", SourceOut: "01:00:01:00",
			DurationFrames: &originalDuration,
		}},
	}

	fixed, _, err := Unify(doc, UnifyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := *fixed.Events[0].DurationFrames; got != 25 {
		t.Fatalf("fixed duration = %d, want 25", got)
	}
	if originalDuration != 24 {
		t.Fatalf("input duration mutated to %d", originalDuration)
	}
}

func hasCode(issues []Issue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}
