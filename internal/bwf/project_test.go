package bwf

import "testing"

func TestParseProjectFlexibleKeys(t *testing.T) {
	raw := []byte(`{
		"project": {
			"sample_rate": "48000",
			"start_sample": 1728000384,
			"duration_seconds": 5.008,
			"frame_rate": "25"
		}
	}`)
	p, err := parseProject(raw)
	if err != nil {
		t.Fatal(err)
	}
	if p.SampleRate != 48000 {
		t.Fatalf("sampleRate=%d", p.SampleRate)
	}
	if p.StartSample == nil || *p.StartSample != 1728000384 {
		t.Fatalf("startSample=%v", p.StartSample)
	}
	if p.DurationSeconds == nil || *p.DurationSeconds != 5.008 {
		t.Fatalf("durationSeconds=%v", p.DurationSeconds)
	}
	if p.TimecodeRate != "25" {
		t.Fatalf("timecodeRate=%q", p.TimecodeRate)
	}
}
