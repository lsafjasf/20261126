package vttlayout

import "testing"

const sampleVTT = `WEBVTT - sample

NOTE this is a note
spanning lines

REGION
id:side
width:40%
lines:4
regionanchor:100%,100%
viewportanchor:96%,92%
priority:7

cue-1
00:00:01.500 --> 00:00:03,250 region:side align:end
Hello <b>bold</b> &amp; goodbye
second line

00:01:02.000 --> 01:02:03.000 line:-2
Mid
`

func TestParseDocument(t *testing.T) {
	doc, err := Parse(sampleVTT)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Cues) != 2 {
		t.Fatalf("want 2 cues, got %d", len(doc.Cues))
	}
	r := doc.Regions["side"]
	if r == nil {
		t.Fatal("region side missing")
	}
	if r.WidthPct != 40 || r.Lines != 4 || r.Priority != 7 || !r.PrioritySet {
		t.Fatalf("region parse wrong: %+v", r)
	}
	if r.RegionAnchorX != 100 || r.ViewportAnchorY != 92 {
		t.Fatalf("anchor parse wrong: %+v", r)
	}

	c0 := doc.Cues[0]
	if c0.ID != "cue-1" {
		t.Fatalf("cue id = %q", c0.ID)
	}
	if c0.StartMS != 1500 || c0.EndMS != 3250 {
		t.Fatalf("bad times %d..%d", c0.StartMS, c0.EndMS)
	}
	if c0.Settings.Region != "side" || c0.Settings.Align != "end" {
		t.Fatalf("settings wrong: %+v", c0.Settings)
	}
	if c0.Text != "Hello bold & goodbye\nsecond line" {
		t.Fatalf("text decode wrong: %q", c0.Text)
	}
	if doc.Priority(c0) != 7 {
		t.Fatalf("region priority not inherited: %d", doc.Priority(c0))
	}

	c1 := doc.Cues[1]
	if c1.StartMS != 62000 || c1.EndMS != 3723000 {
		t.Fatalf("long timestamp parse wrong: %d..%d", c1.StartMS, c1.EndMS)
	}
	if !c1.Settings.HasLineNumber || c1.Settings.LineNumber != -2 {
		t.Fatalf("line setting wrong: %+v", c1.Settings)
	}
}

func TestTimestampRoundTrip(t *testing.T) {
	cases := []Milliseconds{0, 999, 1500, 61000, 3661500}
	for _, ms := range cases {
		s := FormatTimestamp(ms)
		got, err := ParseTimestamp(s)
		if err != nil {
			t.Fatalf("parse %q: %v", s, err)
		}
		if got != ms {
			t.Fatalf("round trip %d -> %q -> %d", ms, s, got)
		}
	}
	if _, err := ParseTimestamp("nope"); err == nil {
		t.Fatal("expected error for bad timestamp")
	}
}

func TestParseRejectsNonVTT(t *testing.T) {
	if _, err := Parse("this is not vtt\n"); err == nil {
		t.Fatal("expected missing WEBVTT error")
	}
}

func TestMissingRegionWarns(t *testing.T) {
	doc, err := Parse("WEBVTT\n\n00:00:01.000 --> 00:00:02.000 region:ghost\nhi\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Warnings) == 0 {
		t.Fatal("expected missing region warning")
	}
}
