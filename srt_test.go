package safereflow

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseAndWriteSRT(t *testing.T) {
	in := "1\n" +
		"00:00:01,200 --> 00:00:03,400\n" +
		"Hello world\n" +
		"second line\n\n" +
		"2\n" +
		"00:01:02,500 --> 00:01:05,000\n" +
		"Next cue\n\n"
	in = "\uFEFF" + in

	doc, err := ParseSRT(strings.NewReader(in), Canvas{Width: 1920, Height: 1080}, DefaultStyle())
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Cues) != 2 {
		t.Fatalf("cues = %d", len(doc.Cues))
	}
	c0 := doc.Cues[0]
	if c0.ID != "1" || !near(c0.Start, 1.2) || !near(c0.End, 3.4) {
		t.Fatalf("cue0 timing/id wrong: %+v", c0)
	}
	if c0.Text != "Hello world\nsecond line" {
		t.Fatalf("cue0 text = %q", c0.Text)
	}
	if !near(doc.Cues[1].Start, 62.5) {
		t.Fatalf("cue1 start = %v", doc.Cues[1].Start)
	}

	var buf bytes.Buffer
	if err := WriteSRT(&buf, doc); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "00:00:01,200 --> 00:00:03,400") {
		t.Fatalf("missing timecode in:\n%s", out)
	}
	if !strings.Contains(out, "Hello world\nsecond line") {
		t.Fatalf("missing text in:\n%s", out)
	}
	if FormatSRTTime(3661.5) != "01:01:01,500" {
		t.Fatalf("time format = %s", FormatSRTTime(3661.5))
	}

	// Re-parse the written output.
	doc2, err := ParseSRT(strings.NewReader(out), doc.Canvas, DefaultStyle())
	if err != nil || len(doc2.Cues) != 2 {
		t.Fatalf("roundtrip failed: %v cues=%d", err, len(doc2.Cues))
	}
	if doc2.Cues[0].Text != c0.Text {
		t.Fatalf("text roundtrip = %q", doc2.Cues[0].Text)
	}
}

func TestParseSRTMissingTimeline(t *testing.T) {
	_, err := ParseSRT(strings.NewReader("1\nno timecode here\n"), Canvas{100, 100}, DefaultStyle())
	if err == nil {
		t.Fatal("expected parse error for block without time line")
	}
}
