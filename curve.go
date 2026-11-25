package clipfade

import "math"

// Law selects the crossfade gain shape.
type Law string

const (
	// LawAuto selects the law from phase analysis:
	// in-phase -> equal gain, uncorrelated -> equal power,
	// anti-phase -> equal gain plus a cancellation warning (or polarity
	// correction when enabled).
	LawAuto Law = "auto"
	// LawEqualGain is the linear (1-x, x) fade. It reconstructs identical
	// in-phase signals exactly with no midpoint boost.
	LawEqualGain Law = "equal_gain"
	// LawEqualPower is the cosine/sine fade. gOut^2+gIn^2=1, which keeps
	// RMS level constant for uncorrelated signals.
	LawEqualPower Law = "equal_power"
)

// EffectiveLaw resolves LawAuto for a given phase relationship.
func EffectiveLaw(requested Law, phase PhaseRelation, polarityCorrected bool) Law {
	if requested != LawAuto && requested != "" {
		return requested
	}
	switch phase {
	case PhaseInPhase:
		return LawEqualGain
	case PhaseAntiPhase:
		if polarityCorrected {
			return LawEqualGain
		}
		// No gain law can undo coherent cancellation; equal gain keeps the
		// peak envelope bounded and the report raises a warning.
		return LawEqualGain
	default:
		return LawEqualPower
	}
}

// Gains returns the outbound (left, fading to 0) and inbound (right, fading
// to 1) gains at normalized position x in [0,1].
func Gains(law Law, x float64) (out, in float64) {
	if x < 0 {
		x = 0
	} else if x > 1 {
		x = 1
	}
	switch law {
	case LawEqualPower:
		theta := x * math.Pi / 2
		return math.Cos(theta), math.Sin(theta)
	default: // LawEqualGain / linear
		return 1 - x, x
	}
}

// Envelope samples both gain curves over n samples. Position uses n-1 in
// the denominator so endpoints are exactly (1,0) and (0,1).
func Envelope(law Law, n int) (out, in []float64) {
	if n <= 0 {
		return nil, nil
	}
	out = make([]float64, n)
	in = make([]float64, n)
	if n == 1 {
		out[0], in[0] = 1, 1
		return out, in
	}
	for i := 0; i < n; i++ {
		x := float64(i) / float64(n-1)
		out[i], in[i] = Gains(law, x)
	}
	return out, in
}

// Mix renders n crossfaded samples from equal-length source windows a (left,
// fade out) and b (right, fade in). flipRight applies polarity correction.
func Mix(a, b []float64, law Law, flipRight bool) []float64 {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	out := make([]float64, n)
	if n == 0 {
		return out
	}
	for i := 0; i < n; i++ {
		x := 0.0
		if n > 1 {
			x = float64(i) / float64(n-1)
		}
		gOut, gIn := Gains(law, x)
		right := b[i]
		if flipRight {
			right = -right
		}
		out[i] = gOut*a[i] + gIn*right
	}
	return out
}
