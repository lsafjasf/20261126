package sdh

import (
	"strings"
	"testing"
	"time"
)

const sampleSRT = `1
00:00:01,000 --> 00:00:03,000
Hello world.

2
00:00:04,500 --> 00:00:06,250 X1:10 X2:20 Y1:30
Second line
and another.

`

func TestParseSRT(t *testing.T) {
	cues, err := ParseSRT(strings.NewReader(sampleSRT))
	if err != nil {
		t.Fatal(err)
	}
	if len(cues) != 2 {
		t.Fatalf("len(cues) = %d, want 2", len(cues))
	}
	if cues[0].ID != "1" || cues[0].Index != 1 {
		t.Fatalf("cue0 id/index = %q/%d", cues[0].ID, cues[0].Index)
	}
	if cues[0].Start != time.Second || cues[0].End != 3*time.Second {
		t.Fatalf("cue0 timing = %v..%v", cues[0].Start, cues[0].End)
	}
	if cues[1].Start != 4500*time.Millisecond || cues[1].End != 6250*time.Millisecond {
		t.Fatalf("cue1 timing = %v..%v", cues[1].Start, cues[1].End)
	}
	if cues[1].Text != "Second line\nand another." {
		t.Fatalf("cue1 text = %q", cues[1].Text)
	}
}

func TestParseSRTBOMAndCRLF(t *testing.T) {
	data := "\xef\xbb\xbf1\r\n00:00:01,000 --> 00:00:02,000\r\nBonjour.\r\n"
	cues, err := ParseSRTBytes([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(cues) != 1 || cues[0].Text != "Bonjour." {
		t.Fatalf("cues = %+v", cues)
	}
}

func TestParseSRTMissingIndex(t *testing.T) {
	data := "00:00:01,000 --> 00:00:02,000\nNo number.\n\n00:00:03,000 --> 00:00:04,000\nAgain.\n"
	cues, err := ParseSRT(strings.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(cues) != 2 || cues[0].Index != 1 || cues[1].Index != 2 {
		t.Fatalf("cues = %+v", cues)
	}
}

func TestRenderSRTRoundTrip(t *testing.T) {
	cues, err := ParseSRT(strings.NewReader(sampleSRT))
	if err != nil {
		t.Fatal(err)
	}
	out := RenderSRT(cues)
	back, err := ParseSRTBytes(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(back) != len(cues) {
		t.Fatalf("round trip count %d != %d", len(back), len(cues))
	}
	for i := range cues {
		if back[i].Start != cues[i].Start || back[i].End != cues[i].End || back[i].Text != cues[i].Text {
			t.Fatalf("cue %d mismatch:\n got %+v\nwant %+v", i, back[i], cues[i])
		}
	}
	if !strings.Contains(string(out), "00:00:01,000 --> 00:00:03,000") {
		t.Fatalf("render missing timing:\n%s", out)
	}
}

func TestParseWebVTTTolerated(t *testing.T) {
	data := "WEBVTT - test\n\nNOTE this is a note\n\n00:00:01.000 --> 00:00:02.500\n♪ Hello ♪\n\n"
	cues, err := ParseSRT(strings.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(cues) != 1 {
		t.Fatalf("cues = %+v", cues)
	}
	if cues[0].Text != "♪ Hello ♪" {
		t.Fatalf("text = %q", cues[0].Text)
	}
	if cues[0].End != 2500*time.Millisecond {
		t.Fatalf("end = %v", cues[0].End)
	}
}
