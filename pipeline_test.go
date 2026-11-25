package sdh

import "testing"

func TestProcessSRTFacade(t *testing.T) {
	data := []byte("1\n00:00:01,000 --> 00:00:03,000\nJOHN (V.O.): Remember.\n♪ Na na na ♪\n\n" +
		"2\n00:00:02,000 --> 00:00:04,000\n[thunder]\n")
	res, err := ProcessSRT(data, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if res.Project == nil || res.Report == nil {
		t.Fatal("missing project/report")
	}
	if !res.Verification.Complete || res.Verification.VocalUnits != 2 {
		t.Fatalf("verification = %+v", res.Verification)
	}
	// Cue 2 is SDH-only -> plain track has one cue.
	if len(res.Plain) != 1 {
		t.Fatalf("plain cues = %d", len(res.Plain))
	}
	if res.Report.PlainCueCount != 1 || res.Report.CueCount != 2 {
		t.Fatalf("report counts = %+v", res.Report)
	}
}
