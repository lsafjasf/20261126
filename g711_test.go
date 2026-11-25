package wavfix_test

import (
	"math"
	"testing"

	"wavfix"
)

func TestG711DecoderRangeAndSilence(t *testing.T) {
	if got := wavfix.DecodeMuLaw(0xff); got != 0 {
		t.Fatalf("μ-law 0xff decoded to %v, want exact zero", got)
	}
	for code := 0; code < 256; code++ {
		mu := wavfix.DecodeMuLaw(byte(code))
		a := wavfix.DecodeALaw(byte(code))
		if math.Abs(mu) > 1 || math.Abs(a) > 1 {
			t.Fatalf("code %d outside normalized range: mu=%v a=%v", code, mu, a)
		}
	}
}

func TestStereoFloatDurationDividesInterleavedValuesByChannels(t *testing.T) {
	got := wavfix.FloatDuration(2000, 2, 8000)
	if got != 0.125 {
		t.Fatalf("duration = %v, want 0.125", got)
	}
	if naive := float64(2000) / 8000; naive != 0.25 {
		t.Fatalf("naive duration = %v, want reproduction value 0.25", naive)
	}
}
