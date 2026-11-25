package clipfade

import (
	"fmt"
	"math"
	"time"
)

// Anchor places the D-sample crossfade region relative to the cut point.
type Anchor string

const (
	// AnchorCenter centers the transition on the cut: [cut-D/2, cut+D/2).
	// Requires D/2 samples of handle on both sides.
	AnchorCenter Anchor = "center"
	// AnchorStartAtCut starts the transition at the cut: [cut, cut+D).
	// Requires a D-sample tail handle on the left clip.
	AnchorStartAtCut Anchor = "start_at_cut"
	// AnchorEndAtCut ends the transition at the cut: [cut-D, cut).
	// Requires a D-sample pre-roll handle on the right clip.
	AnchorEndAtCut Anchor = "end_at_cut"
)

// PolarityMode decides how anti-phase coherence is handled.
type PolarityMode int

const (
	// PolarityPreserve never changes polarity; anti-phase material produces
	// a cancellation warning and is measured by the peak/dip checks.
	PolarityPreserve PolarityMode = iota
	// PolarityCorrectAntiPhase flips the right side when analysis proves
	// strong anti-phase coherence, converting it to in-phase coherence.
	PolarityCorrectAntiPhase
)

// ClipFade is the clip-level crossfade model. It is bound to two clip IDs,
// not to absolute automation points. Absolute curves are derived values and
// are discarded/rebuilt whenever clip geometry changes.
type ClipFade struct {
	ID     FadeID
	Left   ClipID
	Right  ClipID
	Law    Law
	Anchor Anchor

	// Duration is the transition length. Used when adding through Model;
	// DurationSamples is the sample-accurate resolved form.
	Duration        time.Duration
	DurationSamples int64

	// Segments is the number of control-point intervals per automation lane.
	// The derived envelope is always rendered sample-accurately regardless.
	Segments int

	// Ceiling for the peak check (0 defaults to 1.0 dBFS sample peak).
	Ceiling float64

	// Polarity selects anti-phase handling.
	Polarity PolarityMode

	// PhaseConfig tunes correlation analysis.
	Phase PhaseConfig
}

// FadePlan is the resolved, immutable derivation for one revision.
type FadePlan struct {
	FadeID          FadeID
	Left            ClipID
	Right           ClipID
	Cut             int64
	RegionStart     int64
	RegionEnd       int64
	DurationSamples int
	Anchor          Anchor
	RequestedLaw    Law
	Law             Law
	FlipRight       bool
	Phase           PhaseReport
	Warnings        []string
}

// FadeReport bundles the derivation with signal checks.
type FadeReport struct {
	Plan                FadePlan
	Peak                PeakReport
	ZeroCrossings       ZeroCrossingReport
	EditZeroCrossing    EditZeroCrossingReport
	CancellationDepthDB float64
}

const minFadeSamples = 2

// ResolveDuration converts a time duration to samples at sampleRate.
func ResolveDuration(d time.Duration, sampleRate int) int64 {
	if d <= 0 || sampleRate <= 0 {
		return 0
	}
	return int64(math.Round(d.Seconds() * float64(sampleRate)))
}

// FadeRegion returns the half-open absolute region [start, end) for a cut.
func FadeRegion(cut, durationSamples int64, anchor Anchor) (start, end int64) {
	switch anchor {
	case AnchorStartAtCut:
		start = cut
	case AnchorEndAtCut:
		start = cut - durationSamples
	default: // AnchorCenter
		start = cut - durationSamples/2
	}
	return start, start + durationSamples
}

func (f *ClipFade) defaults(sampleRate int) {
	if f.Law == "" {
		f.Law = LawAuto
	}
	if f.Anchor == "" {
		f.Anchor = AnchorCenter
	}
	if f.Ceiling == 0 {
		f.Ceiling = 1
	}
	if f.Segments <= 0 {
		f.Segments = 16
	}
	if f.DurationSamples < minFadeSamples && f.Duration > 0 {
		f.DurationSamples = ResolveDuration(f.Duration, sampleRate)
	}
}

// prepareFade validates geometry and extracts the raw aligned windows.
func prepareFade(f *ClipFade, left, right *Clip, sampleRate int) (FadePlan, []float64, []float64, error) {
	if f == nil {
		return FadePlan{}, nil, nil, ErrNothingToRebuild
	}
	f.defaults(sampleRate)
	if f.DurationSamples < minFadeSamples {
		return FadePlan{}, nil, nil, fmt.Errorf("%w: got %d samples", ErrInvalidDuration, f.DurationSamples)
	}
	if left == nil || right == nil {
		return FadePlan{}, nil, nil, ErrClipNotFound
	}
	if left.Kind == KindVideo || right.Kind == KindVideo {
		return FadePlan{}, nil, nil, fmt.Errorf("%w: crossfade sides must be audio clips", ErrClipsNotAdjacent)
	}

	cut := left.End()
	if right.Start != cut {
		return FadePlan{}, nil, nil, fmt.Errorf("%w: %s ends at %d but %s starts at %d",
			ErrClipsNotAdjacent, left.ID, cut, right.ID, right.Start)
	}

	d := f.DurationSamples
	start, end := FadeRegion(cut, d, f.Anchor)

	a := make([]float64, d)
	b := make([]float64, d)
	for i := int64(0); i < d; i++ {
		t := start + i
		av, okA := left.sampleAt(t)
		bv, okB := right.sampleAt(t)
		if !okA {
			return FadePlan{}, nil, nil, fmt.Errorf("%w: %s at timeline %d (media index %d)",
				ErrInsufficientHandle, left.ID, t, left.sourceIndex(t))
		}
		if !okB {
			return FadePlan{}, nil, nil, fmt.Errorf("%w: %s at timeline %d (media index %d)",
				ErrInsufficientHandle, right.ID, t, right.sourceIndex(t))
		}
		a[i] = av
		b[i] = bv
	}

	phase, err := AnalyzePhase(a, b, sampleRate, f.Phase)
	if err != nil {
		return FadePlan{}, nil, nil, err
	}

	flip := f.Polarity == PolarityCorrectAntiPhase && phase.Relation == PhaseAntiPhase
	law := EffectiveLaw(f.Law, phase.Relation, flip)

	var warnings []string
	if phase.Relation == PhaseAntiPhase && !flip {
		warnings = append(warnings,
			"anti-phase coherence causes cancellation inside the overlap; consider polarity correction or a zero-crossing butt cut")
	}

	plan := FadePlan{
		FadeID:          f.ID,
		Left:            f.Left,
		Right:           f.Right,
		Cut:             cut,
		RegionStart:     start,
		RegionEnd:       end,
		DurationSamples: int(d),
		Anchor:          f.Anchor,
		RequestedLaw:    f.Law,
		Law:             law,
		FlipRight:       flip,
		Phase:           phase,
		Warnings:        warnings,
	}
	return plan, a, b, nil
}

// BuildPlan resolves geometry and phase analysis without rendering.
func BuildPlan(f *ClipFade, left, right *Clip, sampleRate int) (FadePlan, error) {
	plan, _, _, err := prepareFade(f, left, right, sampleRate)
	return plan, err
}

// RenderFade renders the sample-accurate crossfade and runs peak,
// zero-crossing and cancellation checks.
func RenderFade(f *ClipFade, left, right *Clip, sampleRate int) ([]float64, FadeReport, error) {
	plan, a, b, err := prepareFade(f, left, right, sampleRate)
	if err != nil {
		return nil, FadeReport{}, err
	}

	mixed := Mix(a, b, plan.Law, plan.FlipRight)
	peak := CheckPeak(mixed, f.Ceiling)
	zc := AnalyzeZeroCrossings(mixed)
	editZC := CheckEditZeroCrossings(left, right, plan.Cut, int64(plan.DurationSamples/2))
	dip := cancellationDepthDB(mixed, plan.DurationSamples)
	if dip < -6 && plan.Phase.Relation == PhaseAntiPhase && !plan.FlipRight {
		plan.Warnings = append(plan.Warnings,
			fmt.Sprintf("measured cancellation notch depth %.1f dB", dip))
	}

	report := FadeReport{
		Plan:                plan,
		Peak:                peak,
		ZeroCrossings:       zc,
		EditZeroCrossing:    editZC,
		CancellationDepthDB: dip,
	}
	return mixed, report, nil
}

// cancellationDepthDB compares the central window RMS with the edge-window
// RMS. Strong negative values indicate a mid-overlap cancellation notch.
func cancellationDepthDB(x []float64, n int) float64 {
	span := n / 8
	if span < 1 {
		span = 1
	}
	edge := (rms(x[:span]) + rms(x[n-span:])) / 2
	center := rms(x[n/2-span/2 : n/2+span/2])
	const eps = 1e-12
	return 20 * math.Log10((center+eps)/(edge+eps))
}

func rms(x []float64) float64 {
	if len(x) == 0 {
		return 0
	}
	var sum float64
	for _, v := range x {
		sum += v * v
	}
	return math.Sqrt(sum / float64(len(x)))
}
