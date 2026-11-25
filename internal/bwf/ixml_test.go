package bwf

import "testing"

func TestParseIXMLResolvesRateAfterStartTime(t *testing.T) {
	raw := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<BWFXML>
  <START_TIME>10:00:00:00</START_TIME>
  <TIMECODE_RATE>25</TIMECODE_RATE>
  <TIMECODE_SAMPLE_RATE>48000</TIMECODE_SAMPLE_RATE>
  <BWF_TIME_REFERENCE>1728000384</BWF_TIME_REFERENCE>
</BWFXML>`)
	x, err := parseIXML(raw)
	if err != nil {
		t.Fatal(err)
	}
	if x.StartTime == nil {
		t.Fatal("START_TIME was not resolved after TIMECODE_RATE")
	}
	rate, err := ParseRate(x.TimecodeRate)
	if err != nil {
		t.Fatal(err)
	}
	pos, err := TimecodeSample(*x.StartTime, rate, x.TimecodeSampleRate)
	if err != nil {
		t.Fatal(err)
	}
	if pos != 1728000000 {
		t.Fatalf("mapped start=%d", pos)
	}
	if x.DirectTimeReference == nil || *x.DirectTimeReference != 1728000384 {
		t.Fatalf("direct reference=%v", x.DirectTimeReference)
	}
}
