package clipfade

import (
	"math"
	"testing"
	"time"
)

func TestResolve200ms(t *testing.T) {
	if got := ResolveDuration(200*time.Millisecond, 48000); got != 9600 {
		t.Fatalf("200ms@48k = %d, want 9600", got)
	}
	if got := ResolveDuration(200*time.Millisecond, 44100); got != 8820 {
		t.Fatalf("200ms@44.1k = %d, want 8820", got)
	}
}

func TestFadeRegionAnchors(t *testing.T) {
	const cut, d = int64(48000), int64(9600)
	cases := []struct {
		anchor    Anchor
		wantStart int64
		wantEnd   int64
	}{
		{AnchorCenter, 43200, 52800},
		{AnchorStartAtCut, 48000, 57600},
		{AnchorEndAtCut, 38400, 48000},
	}
	for _, tc := range cases {
		start, end := FadeRegion(cut, d, tc.anchor)
		if start != tc.wantStart || end != tc.wantEnd {
			t.Fatalf("%s region = [%d,%d), want [%d,%d)", tc.anchor, start, end, tc.wantStart, tc.wantEnd)
		}
	}
}

func TestAnalyzePhaseClassifications(t *testing.T) {
	n := testFadeD
	gen := func(wave func(int64) float64) []float64 {
		out := make([]float64, n)
		for i := range out {
			out[i] = wave(int64(testHalfD + i))
		}
		return out
	}

	in := gen(sineAt(440, 0.7))
	antiSig := make([]float64, n)
	for i, v := range in {
		antiSig[i] = -v
	}
	uncorrA := gen(noiseWave(1))
	uncorrB := gen(noiseWave(2))

	cases := []struct {
		name string
		a, b []float64
		want PhaseRelation
	}{
		{"in-phase", in, in, PhaseInPhase},
		{"anti-phase", in, antiSig, PhaseAntiPhase},
		{"uncorrelated-noise", uncorrA, uncorrB, PhaseUncorrelated},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rep, err := AnalyzePhase(tc.a, tc.b, testSR, PhaseConfig{MaxLag: -1})
			if err != nil {
				t.Fatal(err)
			}
			if rep.Relation != tc.want {
				t.Fatalf("relation = %s (rho=%.3f), want %s", rep.Relation, rep.Correlation, tc.want)
			}
		})
	}
}

func TestAnalyzePhaseFindsLag(t *testing.T) {
	n := 4800
	src := make([]float64, n)
	for i := range src {
		src[i] = math.Sin(2 * math.Pi * 330 * float64(i) / testSR)
	}
	const shift = 37
	shifted := make([]float64, n)
	for i := shift; i < n; i++ {
		shifted[i] = src[i-shift]
	}
	rep, err := AnalyzePhase(src, shifted, testSR, PhaseConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Relation != PhaseInPhase {
		t.Fatalf("expected coherent after lag search, got %s rho=%.3f", rep.Relation, rep.Correlation)
	}
	if got := absInt(rep.LagSamples); absInt(got-shift) > 1 {
		t.Fatalf("|lag| = %d (lag=%d), want ~%d", got, rep.LagSamples, shift)
	}
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func TestEqualGainReconstructsInPhase(t *testing.T) {
	n := 256
	a := make([]float64, n)
	for i := range a {
		a[i] = 0.5 * math.Sin(float64(i)*0.3)
	}
	mixed := Mix(a, a, LawEqualGain, false)
	for i, v := range mixed {
		if math.Abs(v-a[i]) > 1e-12 {
			t.Fatalf("sample %d = %.12f, want %.12f", i, v, a[i])
		}
	}
}

func TestEqualPowerSumsSquaresToOne(t *testing.T) {
	out, in := Envelope(LawEqualPower, 513)
	for i := range out {
		s := out[i]*out[i] + in[i]*in[i]
		if math.Abs(s-1) > 1e-12 {
			t.Fatalf("i=%d gOut^2+gIn^2=%.12f", i, s)
		}
	}
	if math.Abs(out[0]-1) > 1e-12 || math.Abs(in[len(in)-1]-1) > 1e-12 {
		t.Fatal("envelope endpoints wrong")
	}
}

func TestCheckPeak(t *testing.T) {
	rep := CheckPeak([]float64{0.2, -0.9, 1.05, 0.1}, 1.0)
	if rep.Peak != 1.05 || rep.PeakIndex != 2 || rep.OverCeiling != 1 || rep.Pass {
		t.Fatalf("unexpected peak report: %+v", rep)
	}
	if CheckPeak(nil, 1).Pass != true {
		t.Fatal("empty buffer should pass")
	}
}

func TestNearestZeroCrossing(t *testing.T) {
	// Exact pattern: crossing right-indices at multiples of 4.
	buf := make([]float64, 40)
	for i := range buf {
		buf[i] = zcPattern(int64(i))
	}
	idx, dist, ok := NearestZeroCrossing(buf, 10, 6)
	if !ok {
		t.Fatal("expected crossing")
	}
	if idx != 8 || dist != 2 {
		t.Fatalf("nearest to 10 = idx %d dist %d, want 8/2", idx, dist)
	}
	if _, _, ok := NearestZeroCrossing(buf, 10, 1); ok {
		t.Fatal("did not expect crossing within 1")
	}
}
