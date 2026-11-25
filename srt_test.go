package cuemap

import (
	"os"
	"testing"
)

func TestParseGeneratedSRT(t *testing.T) {
	data, err := os.ReadFile("testdata/trilingual.srt")
	if err != nil {
		t.Fatalf("read SRT: %v", err)
	}
	cues, err := ParseSRT(string(data))
	if err != nil {
		t.Fatalf("ParseSRT: %v", err)
	}
	if len(cues) != 6 {
		t.Fatalf("cue count = %d, want 6", len(cues))
	}

	first := cues[0]
	if first.StartMS != 0 || first.EndMS != 2500 {
		t.Fatalf("timing = %d..%d", first.StartMS, first.EndMS)
	}
	if got := MapLine(first.Text, DirectionAuto); got.BaseDirection != DirectionRTL {
		t.Fatalf("first cue direction = %v, want RTL", got.BaseDirection)
	}

	trilingual := cues[1]
	if trilingual.ID != "2" || len(trilingual.Ruby) != 1 {
		t.Fatalf("trilingual cue = %+v, want one ruby span", trilingual)
	}
	result := Typeset(trilingual, Options{Mode: ModeVertical})
	if !result.OK {
		t.Fatalf("SRT trilingual layout failed: %+v", result.Fallback)
	}
	if trilingual.Raw == "" || trilingual.Raw[0] == '\n' {
		t.Fatalf("raw block not preserved: %q", trilingual.Raw)
	}
}

func TestParseSRTTimingError(t *testing.T) {
	_, err := ParseSRT("1\nnot-a-timing\ntext\n")
	if err == nil {
		t.Fatal("expected timing parse error")
	}
}
