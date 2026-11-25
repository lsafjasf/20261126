package main

import (
	"fmt"
	"io"
	"strings"

	"edlframe"
)

func renderText(w io.Writer, report *edlframe.Report) {
	fmt.Fprintf(w, "EDL framebase report: %d reel(s), %d event(s)\n", report.Summary.ReelCount, report.Summary.EventCount)
	fmt.Fprintf(
		w,
		"summary: reel_conflicts=%d event_conflicts=%d invalid=%d auto_convertible=%d affected_frames=%d\n\n",
		report.Summary.ReelConflictCount,
		report.Summary.EventConflictCount,
		report.Summary.InvalidEventCount,
		report.Summary.ConvertedEventCount,
		report.Summary.AffectedFrameCount,
	)

	for _, reel := range report.Reels {
		conflict := "ok"
		if reel.RateConflict {
			conflict = "REEL CONFLICT"
		}
		fmt.Fprintf(w, "[reel %s] %s source=%s event_table=%s canonical=%s\n", reel.ReelID, conflict, orDash(reel.SourceListRate), orDash(reel.EventTableRate), orDash(reel.CanonicalRate))
		for _, issue := range reel.Issues {
			fmt.Fprintf(w, "  - %s %s: %s\n", issue.Severity, issue.Code, issue.Message)
		}
		if len(reel.ConflictEventIDs) > 0 {
			fmt.Fprintf(w, "  event conflicts: %s\n", strings.Join(reel.ConflictEventIDs, ", "))
		}
		if len(reel.InvalidEventIDs) > 0 {
			fmt.Fprintf(w, "  invalid/manual: %s\n", strings.Join(reel.InvalidEventIDs, ", "))
		}
		if reel.AffectedFrameCount > 0 {
			fmt.Fprintf(w, "  unified affected frames: %d\n", reel.AffectedFrameCount)
		}
		fmt.Fprintln(w)
	}

	for _, event := range report.Events {
		if !event.RateConflict && !hasError(event.Issues) {
			continue
		}
		badge := "event conflict"
		if !event.RateConflict {
			badge = "invalid"
		}
		fmt.Fprintf(w, "[event %s reel=%s] %s: %s -> %s convertible=%t\n", event.EventID, event.ReelID, badge, orDash(event.DeclaredRate), orDash(event.CanonicalRate), event.Convertible)
		renderInterval(w, "source", event.Source)
		renderInterval(w, "record", event.Record)
		for _, issue := range event.Issues {
			fmt.Fprintf(w, "  - %s %s: %s\n", issue.Severity, issue.Code, issue.Message)
		}
		fmt.Fprintln(w)
	}
}

func renderInterval(w io.Writer, name string, interval *edlframe.IntervalReport) {
	if interval == nil {
		return
	}
	fmt.Fprintf(w, "  %s: %s -> %s duration=%d frames valid=%t", name, interval.RawIn, interval.RawOut, interval.DurationFrames, interval.Valid)
	if interval.CrossesMidnight {
		fmt.Fprint(w, " crosses_midnight")
	}
	fmt.Fprintln(w)
	if interval.Impact == nil {
		return
	}
	declared := interval.Impact.Declared
	canonical := interval.Impact.Canonical
	fmt.Fprintf(
		w,
		"    declared  [%d,%d) %s..%s (%d frames)\n",
		declared.StartFrame, declared.EndFrame, declared.StartTimecode, declared.EndTimecode, declared.FrameCount,
	)
	fmt.Fprintf(
		w,
		"    canonical [%d,%d) %s..%s (%d frames); affected %d..%d\n",
		canonical.StartFrame, canonical.EndFrame, canonical.StartTimecode, canonical.EndTimecode, canonical.FrameCount,
		interval.Impact.AffectedFrames.First, interval.Impact.AffectedFrames.Last,
	)
}

func orDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func hasError(issues []edlframe.Issue) bool {
	for _, issue := range issues {
		if issue.Severity == edlframe.SeverityError {
			return true
		}
	}
	return false
}
