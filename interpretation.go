package fourch

import (
	"fmt"
	"math"
	"sort"
)

// DefaultResidualTolerance is the maximum Euclidean distance between two
// normalized four-channel vectors for a candidate to count as supported.
const DefaultResidualTolerance = 0.08

// Observation is one result from rotating a known source. Channels are the
// measured values for stream channels 0..3 after arbitrary recording gain.
// The direction describes the source at that observation.
type Observation struct {
	AzimuthDeg   float64
	ElevationDeg float64
	Channels     [4]float64
}

// FitStatus says whether the evidence supports a candidate.
type FitStatus string

const (
	FitSupported    FitStatus = "supported"
	FitUnsupported  FitStatus = "unsupported"
	FitInsufficient FitStatus = "insufficient-evidence"
)

// CandidateFit contains the residual of one interpretation against all usable
// observations.
type CandidateFit struct {
	Candidate        Candidate
	Status           FitStatus
	Residual         float64
	MaxResidual      float64
	UsedObservations int
}

// Interpretation does not force a single answer. Plausible contains every
// candidate supported by the evidence; Ambiguous is true when that set is not
// a singleton. Best is set only when exactly one candidate is supported.
type Interpretation struct {
	Fits          []CandidateFit
	Plausible     []CandidateFit
	Best          *Candidate
	Ambiguous     bool
	IgnoredSilent int
	Reason        string
}

// InterpretOptions adjusts evidence matching. The zero value is valid.
type InterpretOptions struct {
	// ResidualTolerance defaults to DefaultResidualTolerance when zero.
	ResidualTolerance float64
}

// Interpret compares normalized channel vectors against each candidate's ideal
// response for the same known source direction. Both sides are normalized, so
// container or recording gain cannot favor one interpretation.
//
// A silent observation contributes no directional evidence. With fewer than one
// usable observation, every candidate is returned as insufficient rather than
// guessing from the "4ch" label.
func Interpret(observations []Observation, candidates []Candidate, opts InterpretOptions) (Interpretation, error) {
	if len(candidates) == 0 {
		return Interpretation{}, fmt.Errorf("no candidates supplied")
	}
	tolerance := opts.ResidualTolerance
	if tolerance == 0 {
		tolerance = DefaultResidualTolerance
	}
	if !isFinite(tolerance) || tolerance < 0 {
		return Interpretation{}, fmt.Errorf("residual tolerance must be finite and non-negative")
	}

	type sample struct {
		observed [4]float64
		az, el   float64
	}
	samples := make([]sample, 0, len(observations))
	ignoredSilent := 0
	for i, obs := range observations {
		if !isFinite(obs.AzimuthDeg) || !isFinite(obs.ElevationDeg) {
			return Interpretation{}, fmt.Errorf("observation %d has a non-finite direction", i)
		}
		for _, channel := range obs.Channels {
			if !isFinite(channel) {
				return Interpretation{}, fmt.Errorf("observation %d has a non-finite channel value", i)
			}
		}
		u, ok := unit4(obs.Channels)
		if !ok {
			ignoredSilent++
			continue
		}
		samples = append(samples, sample{u, obs.AzimuthDeg, obs.ElevationDeg})
	}

	out := Interpretation{
		Fits:          make([]CandidateFit, 0, len(candidates)),
		IgnoredSilent: ignoredSilent,
	}
	for _, candidate := range candidates {
		fit := CandidateFit{Candidate: candidate}
		if len(samples) == 0 {
			fit.Status = FitInsufficient
			out.Fits = append(out.Fits, fit)
			continue
		}
		fit.Status = FitSupported
		for _, s := range samples {
			predicted, err := candidate.Predict(s.az, s.el)
			if err != nil {
				return Interpretation{}, fmt.Errorf("predict %q: %w", candidate.Label, err)
			}
			v, ok := unit4(predicted)
			if !ok {
				return Interpretation{}, fmt.Errorf("candidate %q has a zero prediction", candidate.Label)
			}
			residual := distance4(s.observed, v)
			fit.Residual += residual * residual
			if residual > fit.MaxResidual {
				fit.MaxResidual = residual
			}
			fit.UsedObservations++
		}
		fit.Residual = math.Sqrt(fit.Residual / float64(fit.UsedObservations))
		if fit.MaxResidual > tolerance {
			fit.Status = FitUnsupported
		}
		out.Fits = append(out.Fits, fit)
	}

	sort.SliceStable(out.Fits, func(i, j int) bool {
		a, b := out.Fits[i], out.Fits[j]
		if statusRank(a.Status) != statusRank(b.Status) {
			return statusRank(a.Status) < statusRank(b.Status)
		}
		return a.Residual < b.Residual
	})

	if len(samples) == 0 {
		out.Plausible = append(out.Plausible, out.Fits...)
		out.Ambiguous = len(out.Plausible) > 1
		out.Reason = "no usable directional evidence; all candidates remain possible"
		return out, nil
	}

	for _, fit := range out.Fits {
		if fit.Status == FitSupported {
			out.Plausible = append(out.Plausible, fit)
		}
	}
	switch len(out.Plausible) {
	case 0:
		out.Ambiguous = false
		out.Reason = "no candidate matches the observations within tolerance"
	case 1:
		best := out.Plausible[0].Candidate
		out.Best = &best
		out.Reason = "one candidate matches the observations within tolerance"
	default:
		out.Ambiguous = true
		out.Reason = "multiple candidates match the observations; more rotations are needed"
	}
	return out, nil
}

func unit4(v [4]float64) ([4]float64, bool) {
	var sum float64
	for _, x := range v {
		sum += x * x
	}
	norm := math.Sqrt(sum)
	if norm < 1e-12 {
		return [4]float64{}, false
	}
	for i := range v {
		v[i] /= norm
	}
	return v, true
}

func distance4(a, b [4]float64) float64 {
	var sum float64
	for i := range a {
		d := a[i] - b[i]
		sum += d * d
	}
	return math.Sqrt(sum)
}

func statusRank(status FitStatus) int {
	switch status {
	case FitSupported:
		return 0
	case FitInsufficient:
		return 1
	default:
		return 2
	}
}
