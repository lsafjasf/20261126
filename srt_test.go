package opusalign

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseSRTConvertsTo48KSamples(t *testing.T) {
	data := "1\n00:00:01,000 --> 00:00:01,500\n你好\n\n2\n00:00:02,250 --> 00:00:03,000\n世界\n"
	cues, err := ParseSRT(strings.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(cues) != 2 {
		t.Fatalf("cues = %d", len(cues))
	}
	if cues[0].StartSample != 48000 || cues[0].EndSample != 72000 {
		t.Fatalf("first cue samples = %d..%d", cues[0].StartSample, cues[0].EndSample)
	}
	if cues[1].StartSample != 108000 {
		t.Fatalf("second start = %d", cues[1].StartSample)
	}
}

func TestWriteSRTSkipsEmptyLostCue(t *testing.T) {
	var output bytes.Buffer
	err := WriteSRT(&output, []SyncedCue{
		{Range: SampleRange{0, 480}, Text: "保留"},
		{Range: SampleRange{480, 960}, Text: ""},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(output.String(), "-->") != 1 {
		t.Fatalf("empty cue should be omitted: %q", output.String())
	}
}
