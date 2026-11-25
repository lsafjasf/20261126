package fourch

import (
	"math"
	"testing"
)

func TestInterpretResolvesCandidateFromGeneratedRotations(t *testing.T) {
	cases := []struct {
		name      string
		candidate Candidate
	}{
		{"quadraphonic", QuadraphonicCandidate()},
		{"ambisonic", AmbisonicCandidate()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			observations, err := generatedRotationObservations(tc.candidate, 30, 12)
			if err != nil {
				t.Fatal(err)
			}
			got, err := Interpret(observations, DefaultCandidates(), InterpretOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if got.Best == nil {
				t.Fatalf("expected one interpretation, got %s (fits %+v)", got.Reason, got.Fits)
			}
			if got.Best.Kind != tc.candidate.Kind {
				t.Fatalf("best = %s, want %s", got.Best.Kind, tc.candidate.Kind)
			}
			if got.Ambiguous {
				t.Fatal("resolved fit marked ambiguous")
			}
			if len(got.Plausible) != 1 {
				t.Fatalf("plausible candidates = %d, want 1", len(got.Plausible))
			}
		})
	}
}

func TestKnownRotationsResolveAmbisonic(t *testing.T) {
	ambi := AmbisonicCandidate()
	left, err := ambi.Predict(90, 0)
	if err != nil {
		t.Fatal(err)
	}
	front, err := ambi.Predict(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	obs := []Observation{
		{AzimuthDeg: 90, Channels: left},
		{AzimuthDeg: 0, Channels: front},
	}
	got, err := Interpret(obs, DefaultCandidates(), InterpretOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Best == nil || got.Best.Kind != KindAmbisonicACNSN3D {
		t.Fatalf("best = %+v, want Ambisonic; fits %+v", got.Best, got.Fits)
	}
}

func TestCoordinateConventionAmbiguityNeedsVerticalEvidence(t *testing.T) {
	canonical := AmbisonicCandidate()
	flippedZ := AmbisonicCandidate()
	flippedZ.Label = "ACN/SN3D with inverted Z convention"
	flippedZ.Channels[2].Vector = Vec3{Z: -1}

	horizontal, err := generatedRotationObservations(canonical, 45, 8)
	if err != nil {
		t.Fatal(err)
	}
	for i := range horizontal {
		horizontal[i].ElevationDeg = 0
		horizontal[i].Channels, err = canonical.Predict(horizontal[i].AzimuthDeg, 0)
		if err != nil {
			t.Fatal(err)
		}
	}
	got, err := Interpret(horizontal, []Candidate{canonical, flippedZ}, InterpretOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Ambiguous || got.Best != nil || len(got.Plausible) != 2 {
		t.Fatalf("horizontal rotations must preserve both Z conventions: %+v", got)
	}

	elevated, err := generatedRotationObservations(canonical, 45, 8)
	if err != nil {
		t.Fatal(err)
	}
	got, err = Interpret(elevated, []Candidate{canonical, flippedZ}, InterpretOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Best == nil || got.Best.Label != canonical.Label {
		t.Fatalf("vertical evidence should resolve the Z convention: %+v", got)
	}
}

func TestSilenceIsInsufficientEvidence(t *testing.T) {
	cases := []struct {
		name string
		obs  []Observation
	}{
		{"no observations", nil},
		{"silent observation", []Observation{{AzimuthDeg: 20, Channels: [4]float64{}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Interpret(tc.obs, DefaultCandidates(), InterpretOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if !got.Ambiguous || got.Best != nil {
				t.Fatalf("silence should stay ambiguous: %+v", got)
			}
			if len(got.Plausible) != 2 {
				t.Fatalf("plausible candidates = %d, want 2", len(got.Plausible))
			}
			for _, fit := range got.Plausible {
				if fit.Status != FitInsufficient {
					t.Errorf("%s status = %s, want insufficient", fit.Candidate.Kind, fit.Status)
				}
			}
		})
	}
}

func TestInterpretRejectsInvalidInput(t *testing.T) {
	obs := []Observation{{AzimuthDeg: math.Inf(1)}}
	if _, err := Interpret(obs, DefaultCandidates(), InterpretOptions{}); err == nil {
		t.Fatal("expected an error for a non-finite direction")
	}
	if _, err := Interpret(nil, nil, InterpretOptions{}); err == nil {
		t.Fatal("expected an error for no candidates")
	}
	if _, err := Interpret(nil, DefaultCandidates(), InterpretOptions{ResidualTolerance: -1}); err == nil {
		t.Fatal("expected an error for a negative tolerance")
	}
}

func generatedRotationObservations(c Candidate, stepDeg float64, count int) ([]Observation, error) {
	out := make([]Observation, 0, count)
	for i := 0; i < count; i++ {
		az := -165 + float64(i)*stepDeg
		el := float64((i%3)-1) * 10
		channels, err := c.Predict(az, el)
		if err != nil {
			return nil, err
		}
		gain := 0.2 + 0.05*float64(i)
		for j := range channels {
			channels[j] *= gain
		}
		out = append(out, Observation{
			AzimuthDeg:   az,
			ElevationDeg: el,
			Channels:     channels,
		})
	}
	return out, nil
}
