package fourch

import (
	"math"
	"testing"
)

func TestQuadraphonicRoutingIsIdentityAndInvertible(t *testing.T) {
	mapping, err := QuadraphonicToSpeakers(CanonicalQuadSpeakers())
	if err != nil {
		t.Fatal(err)
	}
	for row := 0; row < 4; row++ {
		for col := 0; col < 4; col++ {
			want := 0.0
			if row == col {
				want = 1
			}
			if mapping.Matrix[row][col] != want {
				t.Fatalf("matrix[%d][%d] = %g, want %g", row, col, mapping.Matrix[row][col], want)
			}
		}
	}
	report, err := mapping.Invertibility(0)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Invertible || !report.Injective || report.Rank != 4 {
		t.Fatalf("unexpected invertibility report: %+v", report)
	}
}

func TestQuadraphonicPermutationRoundTripsGeneratedSources(t *testing.T) {
	speakers := []Speaker{
		{Name: "FR", AzimuthDeg: -45},
		{Name: "FL", AzimuthDeg: 45},
		{Name: "BR", AzimuthDeg: -135},
		{Name: "BL", AzimuthDeg: 135},
	}
	mapping, err := QuadraphonicToSpeakers(speakers)
	if err != nil {
		t.Fatal(err)
	}
	report, err := mapping.Invertibility(0)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Invertible {
		t.Fatalf("permutation should be invertible: %s", report.Reason)
	}
	observations, err := generatedRotationObservations(QuadraphonicCandidate(), 45, 8)
	if err != nil {
		t.Fatal(err)
	}
	for i, observation := range observations {
		decoded, err := mapping.Decode(observation.Channels)
		if err != nil {
			t.Fatal(err)
		}
		recovered, err := report.Inverse.Apply(decoded)
		if err != nil {
			t.Fatal(err)
		}
		for channel, want := range observation.Channels {
			if math.Abs(recovered[channel]-want) > 1e-12 {
				t.Fatalf("sample %d channel %d recovered %g, want %g", i, channel, recovered[channel], want)
			}
		}
	}
}

func TestAmbisonicDecodeToPlanarQuadIsRankThree(t *testing.T) {
	mapping, err := AmbisonicToSpeakers(CanonicalQuadSpeakers())
	if err != nil {
		t.Fatal(err)
	}
	report, err := mapping.Invertibility(0)
	if err != nil {
		t.Fatal(err)
	}
	if report.Rank != 3 || report.Injective || report.Invertible {
		t.Fatalf("unexpected report: %+v", report)
	}
	if len(report.Nullspace) != 1 {
		t.Fatalf("nullspace dimension = %d, want 1", len(report.Nullspace))
	}
	witness := report.Nullspace[0]
	want := []float64{0, 0, 1, 0}
	for i := range want {
		if math.Abs(witness[i]-want[i]) > 1e-10 {
			t.Fatalf("nullspace witness = %v, want the ACN2/Z axis", witness)
		}
	}
	lostImage, err := mapping.Matrix.Apply(witness)
	if err != nil {
		t.Fatal(err)
	}
	for i, value := range lostImage {
		if math.Abs(value) > 1e-12 {
			t.Fatalf("lost height component appears at speaker %d: %g", i, value)
		}
	}
}

func TestAmbisonicDecodeCanBeInvertibleWithHeight(t *testing.T) {
	speakers := []Speaker{
		{Name: "front", AzimuthDeg: 0, ElevationDeg: 0},
		{Name: "left", AzimuthDeg: 90, ElevationDeg: 0},
		{Name: "back", AzimuthDeg: 180, ElevationDeg: 0},
		{Name: "up", AzimuthDeg: 0, ElevationDeg: 90},
	}
	mapping, err := AmbisonicToSpeakers(speakers)
	if err != nil {
		t.Fatal(err)
	}
	report, err := mapping.Invertibility(0)
	if err != nil {
		t.Fatal(err)
	}
	if report.Rank != 4 || !report.Invertible {
		t.Fatalf("non-coplanar speaker set should be invertible: %+v", report)
	}
}

func TestMapToSpeakersUsesCandidateCoordinateAxes(t *testing.T) {
	flipped := AmbisonicCandidate()
	flipped.Channels[2].Vector = Vec3{Z: -1}
	mapping, err := MapToSpeakers(flipped, []Speaker{{Name: "up", ElevationDeg: 90}})
	if err != nil {
		t.Fatal(err)
	}
	want := []float64{1, 0, -1, 0}
	for i := range want {
		if math.Abs(mapping.Matrix[0][i]-want[i]) > 1e-12 {
			t.Fatalf("decoder row = %v, want %v", mapping.Matrix[0], want)
		}
	}
}

func TestRectangularAmbisonicMapIsOnlyInjective(t *testing.T) {
	speakers := []Speaker{
		{Name: "front", AzimuthDeg: 0, ElevationDeg: 0},
		{Name: "left", AzimuthDeg: 90, ElevationDeg: 0},
		{Name: "back", AzimuthDeg: 180, ElevationDeg: 0},
		{Name: "up", AzimuthDeg: 0, ElevationDeg: 90},
		{Name: "extra", AzimuthDeg: 30, ElevationDeg: 20},
	}
	mapping, err := AmbisonicToSpeakers(speakers)
	if err != nil {
		t.Fatal(err)
	}
	report, err := mapping.Invertibility(0)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Injective || report.Invertible || report.Square {
		t.Fatalf("unexpected report: %+v", report)
	}
}

func TestQuadraphonicRoutingValidatesRoles(t *testing.T) {
	if _, err := QuadraphonicToSpeakers(nil); err == nil {
		t.Fatal("expected an error for a missing speaker set")
	}
	duplicate := CanonicalQuadSpeakers()
	duplicate[1].Name = "FL"
	if _, err := QuadraphonicToSpeakers(duplicate); err == nil {
		t.Fatal("expected an error for a duplicate speaker role")
	}
	missing := CanonicalQuadSpeakers()[:3]
	if _, err := QuadraphonicToSpeakers(missing); err == nil {
		t.Fatal("expected an error for an incomplete quad layout")
	}
}
