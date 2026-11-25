package clipfade

import (
	"math"
	"time"
)

// PhaseRelation classifies the phase relationship between the two sides of
// a cut using normalized cross-correlation.
type PhaseRelation string

const (
	// PhaseUnknown means analysis was impossible (empty/constant signal).
	PhaseUnknown PhaseRelation = "unknown"
	// PhaseInPhase means strongly positively correlated (rho >= threshold).
	PhaseInPhase PhaseRelation = "in_phase"
	// PhaseAntiPhase means strongly negatively correlated (rho <= -threshold).
	PhaseAntiPhase PhaseRelation = "anti_phase"
	// PhaseUncorrelated means no significant correlation.
	PhaseUncorrelated PhaseRelation = "uncorrelated"
)

// PhaseConfig tunes signal relationship analysis.
type PhaseConfig struct {
	// CorrelationThreshold is the |rho| cutoff for coherent material.
	// Zero defaults to DefaultCorrelationThreshold.
	CorrelationThreshold float64
	// MaxLag bounds alignment search. Zero selects the built-in default of
	// 2 ms; a negative value disables lag search (lag 0 only).
	MaxLag time.Duration
}

// DefaultCorrelationThreshold is the built-in coherence cutoff.
const DefaultCorrelationThreshold = 0.6

func (c PhaseConfig) withDefaults(sampleRate int) PhaseConfig {
	if c.CorrelationThreshold == 0 {
		c.CorrelationThreshold = DefaultCorrelationThreshold
	}
	switch {
	case c.MaxLag < 0:
		c.MaxLag = 0
	case c.MaxLag == 0 && sampleRate > 0:
		c.MaxLag = 2 * time.Millisecond
	}
	return c
}

// PhaseReport is the result of signal relationship analysis.
type PhaseReport struct {
	Relation    PhaseRelation
	Correlation float64
	// LagSamples is the best alignment offset found by correlation search.
	LagSamples int
	Samples    int
}

// AnalyzePhase classifies two equal-length signal windows. It searches a
// bounded lag range for the best normalized correlation so a small timing
// offset does not hide coherence.
func AnalyzePhase(a, b []float64, sampleRate int, cfg PhaseConfig) (PhaseReport, error) {
	cfg = cfg.withDefaults(sampleRate)
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	if n < 4 {
		return PhaseReport{}, ErrEmptyAnalysis
	}
	a = a[:n]
	b = b[:n]

	maxLag := 0
	if cfg.MaxLag > 0 && sampleRate > 0 {
		maxLag = int(math.Round(cfg.MaxLag.Seconds() * float64(sampleRate)))
		if maxLag >= n {
			maxLag = n / 4
		}
	}

	bestRho := -2.0
	bestLag := 0
	// Evaluate lag 0 first-ish; iterate all and keep the largest |rho| so
	// that both strong positive and strong negative coherence are detected
	// even with a small alignment offset.
	for lag := -maxLag; lag <= maxLag; lag++ {
		rho, ok := pearsonAtLag(a, b, lag)
		if !ok {
			continue
		}
		if math.Abs(rho) > math.Abs(bestRho) || bestRho < -1.5 {
			bestRho = rho
			bestLag = lag
		}
	}
	if bestRho < -1.5 {
		// Constant windows make correlation undefined.
		return PhaseReport{Relation: PhaseUnknown, Samples: n}, nil
	}

	rel := PhaseUncorrelated
	switch {
	case bestRho >= cfg.CorrelationThreshold:
		rel = PhaseInPhase
	case bestRho <= -cfg.CorrelationThreshold:
		rel = PhaseAntiPhase
	}
	return PhaseReport{
		Relation:    rel,
		Correlation: clamp1(bestRho),
		LagSamples:  bestLag,
		Samples:     n,
	}, nil
}

// PearsonCorrelation returns the centered normalized correlation of two
// equal-length windows at zero lag.
func PearsonCorrelation(a, b []float64) float64 {
	rho, ok := pearsonAtLag(a, b, 0)
	if !ok {
		return 0
	}
	return rho
}

// pearsonAtLag compares a[k] with b[k-lag] over the overlapping range.
func pearsonAtLag(a, b []float64, lag int) (rho float64, enough bool) {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	var ai, bi int
	if lag >= 0 {
		ai = lag
		bi = 0
	} else {
		ai = 0
		bi = -lag
	}
	m := n - maxInt(ai, bi)
	if m < 4 {
		return 0, false
	}

	var ma, mb float64
	for k := 0; k < m; k++ {
		ma += a[ai+k]
		mb += b[bi+k]
	}
	ma /= float64(m)
	mb /= float64(m)

	var saa, sbb, sab float64
	for k := 0; k < m; k++ {
		x := a[ai+k] - ma
		y := b[bi+k] - mb
		saa += x * x
		sbb += y * y
		sab += x * y
	}
	den := math.Sqrt(saa * sbb)
	if den == 0 {
		return 0, false
	}
	return sab / den, true
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func clamp1(v float64) float64 {
	if v > 1 {
		return 1
	}
	if v < -1 {
		return -1
	}
	return v
}
