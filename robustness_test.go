package vttlayout

import (
	"strings"
	"testing"
)

func TestEmptyDocumentDoesNotPanic(t *testing.T) {
	doc, err := Parse("WEBVTT\n\nSTYLE\n::cue {}\n\nNOTE bye\n")
	if err != nil {
		t.Fatal(err)
	}
	plan := Analyze(doc, Portrait1080x1920(), Options{})
	if len(plan.Boxes) != 0 || !plan.Feasible {
		t.Fatal("empty doc should be feasible with no boxes")
	}
	if KeyFrameTimes(doc) != nil {
		t.Fatal("empty doc should have no key times")
	}
	if RichestFrameTime(doc) != -1 {
		t.Fatal("empty doc richest time should be -1")
	}
	svg := RenderSVGFrame(plan, -1, FrameAfter)
	assertWellFormedXML(t, svg)
	frame := RenderTerminalFrame(plan, 1000, FrameAfter, 60, false)
	if !strings.Contains(frame, "00:00:01.000") {
		t.Fatal("terminal frame should still render timecode")
	}
}

func TestLongUnbreakableTokenStaysNarrow(t *testing.T) {
	vtt := "WEBVTT\n\n00:00:01.000 --> 00:00:04.000\n" +
		"abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvwxyz\n"
	doc, err := Parse(vtt)
	if err != nil {
		t.Fatal(err)
	}
	cv := Portrait1080x1920()
	plan := Analyze(doc, cv, Options{})
	b := plan.Boxes[0]
	if b.Rect.W > cv.SafeRect().W+1 {
		t.Fatalf("box width %.0f exceeds safe width %.0f", b.Rect.W, cv.SafeRect().W)
	}
	for _, ln := range b.Lines {
		if DefaultMeter(cv.FontPx).Measure(ln) > b.Rect.W+1 {
			t.Fatalf("line %q wider than box", ln)
		}
	}
	if !b.PlacementOK {
		t.Fatal("single long token cue should still place")
	}
}

func TestCuePrioritySetting(t *testing.T) {
	vtt := `WEBVTT

00:00:01.000 --> 00:00:05.000 priority:99
high

00:00:01.000 --> 00:00:05.000 priority:-5
low
`
	doc, err := Parse(vtt)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Priority(doc.Cues[0]) != 99 || doc.Priority(doc.Cues[1]) != -5 {
		t.Fatal("cue priority settings not parsed")
	}
	plan := Analyze(doc, Portrait1080x1920(), Options{})
	if len(plan.Boxes) != 2 || len(plan.After) != 0 {
		t.Fatalf("two default bottom cues should stack cleanly: after=%+v", plan.After)
	}
}
