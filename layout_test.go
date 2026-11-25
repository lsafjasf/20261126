package fourch

import (
	"math"
	"testing"
)

func TestAmbisonicChannelRolesAndPredictions(t *testing.T) {
	c := AmbisonicCandidate()
	wantNames := [...]string{"ACN0/W", "ACN1/Y", "ACN2/Z", "ACN3/X"}
	for i, want := range wantNames {
		if c.Channels[i].Index != i {
			t.Fatalf("channel %d index = %d", i, c.Channels[i].Index)
		}
		if c.Channels[i].Name != want {
			t.Errorf("channel %d name = %q, want %q", i, c.Channels[i].Name, want)
		}
	}
	cases := []struct {
		az, el float64
		want   [4]float64
	}{
		{0, 0, [4]float64{1, 0, 0, 1}},
		{90, 0, [4]float64{1, 1, 0, 0}},
		{180, 0, [4]float64{1, 0, 0, -1}},
		{-90, 0, [4]float64{1, -1, 0, 0}},
		{0, 90, [4]float64{1, 0, 1, 0}},
	}
	for _, tc := range cases {
		got, err := c.Predict(tc.az, tc.el)
		if err != nil {
			t.Fatalf("Predict(%g, %g): %v", tc.az, tc.el, err)
		}
		assertVectorClose(t, got, tc.want, 1e-12)
	}
}

func TestQuadraphonicCandidatesHitTheirOwnSpeakers(t *testing.T) {
	c := QuadraphonicCandidate()
	for channel, role := range c.Channels {
		az := math.Atan2(role.Vector.Y, role.Vector.X) * 180 / math.Pi
		got, err := c.Predict(az, 0)
		if err != nil {
			t.Fatalf("Predict(%s): %v", role.Name, err)
		}
		for i := range got {
			want := 0.0
			if i == channel {
				want = 1
			}
			if math.Abs(got[i]-want) > 1e-12 {
				t.Errorf("%s channel %d = %g, want %g", role.Name, i, got[i], want)
			}
		}
	}
	front, err := c.Predict(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	assertVectorClose(t, front, [4]float64{
		1 / math.Sqrt2, 1 / math.Sqrt2, 0, 0,
	}, 1e-12)
}

func TestDirectionUsesAmbiXAxes(t *testing.T) {
	cases := []struct {
		name   string
		az, el float64
		want   Vec3
	}{
		{"front", 0, 0, Vec3{X: 1}},
		{"left", 90, 0, Vec3{Y: 1}},
		{"back", 180, 0, Vec3{X: -1}},
		{"right", -90, 0, Vec3{Y: -1}},
		{"up", 0, 90, Vec3{Z: 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertVec3Close(t, Direction(tc.az, tc.el), tc.want, 1e-12)
		})
	}
}

func TestPredictRejectsInvalidDirection(t *testing.T) {
	c := AmbisonicCandidate()
	if _, err := c.Predict(math.NaN(), 0); err == nil {
		t.Fatal("expected an error for NaN azimuth")
	}
}

func assertVectorClose(t *testing.T, got, want [4]float64, tolerance float64) {
	t.Helper()
	for i := range got {
		if math.Abs(got[i]-want[i]) > tolerance {
			t.Fatalf("vector[%d] = %.15g, want %.15g (got %v, want %v)", i, got[i], want[i], got, want)
		}
	}
}

func assertVec3Close(t *testing.T, got, want Vec3, tolerance float64) {
	t.Helper()
	if math.Abs(got.X-want.X) > tolerance ||
		math.Abs(got.Y-want.Y) > tolerance ||
		math.Abs(got.Z-want.Z) > tolerance {
		t.Fatalf("vector = %+v, want %+v", got, want)
	}
}
