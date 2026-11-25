package clipfade

import "math"

// PeakReport describes sample peaks of a rendered signal. This is a sample
// peak report; oversampled true-peak metering is a deliberate non-goal.
type PeakReport struct {
	// Peak is the maximum absolute sample value.
	Peak float64
	// PeakIndex is the index of Peak in the measured buffer (-1 if empty).
	PeakIndex int
	// Ceiling is the pass threshold.
	Ceiling float64
	// OverCeiling is the count of samples with |value| > Ceiling.
	OverCeiling int
	// Pass is true when no sample exceeds Ceiling.
	Pass bool
}

// CheckPeak measures the sample peak against ceiling (use 1.0 for FS).
func CheckPeak(samples []float64, ceiling float64) PeakReport {
	if ceiling <= 0 {
		ceiling = 1
	}
	rep := PeakReport{PeakIndex: -1, Ceiling: ceiling, Pass: true}
	for i, v := range samples {
		a := math.Abs(v)
		if a > rep.Peak || rep.PeakIndex < 0 {
			rep.Peak = a
			rep.PeakIndex = i
		}
		if a > ceiling {
			rep.OverCeiling++
		}
	}
	rep.Pass = rep.OverCeiling == 0
	return rep
}

// ZeroCrossingReport summarizes zero crossings of a buffer.
type ZeroCrossingReport struct {
	// Count is the number of crossing edges between consecutive samples.
	Count int
	// Indices are the right-hand sample indices of crossing edges.
	Indices []int
	// NearestToStart is the first crossing index, or -1.
	NearestToStart int
	// NearestToEnd is the last crossing index, or -1.
	NearestToEnd int
}

// AnalyzeZeroCrossings records every sign-change edge in samples.
// An exact zero sample counts as a crossing edge with either neighbour.
func AnalyzeZeroCrossings(samples []float64) ZeroCrossingReport {
	rep := ZeroCrossingReport{NearestToStart: -1, NearestToEnd: -1}
	for i := 1; i < len(samples); i++ {
		if isCrossingEdge(samples[i-1], samples[i]) {
			rep.Count++
			rep.Indices = append(rep.Indices, i)
		}
	}
	if len(rep.Indices) > 0 {
		rep.NearestToStart = rep.Indices[0]
		rep.NearestToEnd = rep.Indices[len(rep.Indices)-1]
	}
	return rep
}

func isCrossingEdge(prev, next float64) bool {
	return (prev < 0 && next >= 0) || (prev > 0 && next <= 0) ||
		(prev == 0 && next == 0)
}

// NearestZeroCrossing finds the crossing edge closest to position, searching
// at most maxWindow samples in each direction. Returns the right-hand index
// of the edge and its absolute distance. ok is false when none exists.
func NearestZeroCrossing(samples []float64, position, maxWindow int) (index int, distance int, ok bool) {
	if maxWindow < 0 {
		maxWindow = 0
	}
	bestIdx := -1
	bestDist := maxWindow + 1
	lo := position - maxWindow
	if lo < 1 {
		lo = 1
	}
	hi := position + maxWindow
	if hi >= len(samples) {
		hi = len(samples) - 1
	}
	for i := lo; i <= hi; i++ {
		if !isCrossingEdge(samples[i-1], samples[i]) {
			continue
		}
		d := i - position
		if d < 0 {
			d = -d
		}
		if d < bestDist {
			bestDist = d
			bestIdx = i
		}
	}
	if bestIdx < 0 {
		return 0, 0, false
	}
	return bestIdx, bestDist, true
}

// nearestTimelineZeroCrossing searches a clip's media around a timeline
// coordinate and returns the crossing edge expressed on the timeline.
func nearestTimelineZeroCrossing(c *Clip, timelinePos int64, maxWindow int64) (int64, int64, bool) {
	if c == nil || c.Kind == KindVideo || len(c.Samples) < 2 {
		return 0, 0, false
	}
	center := c.sourceIndex(timelinePos)
	idx, dist, ok := NearestZeroCrossing(c.Samples, int(center), int(maxWindow))
	if !ok {
		return 0, 0, false
	}
	edgeTimeline := c.timelineSample(int64(idx))
	return edgeTimeline, int64(dist), true
}

// FindJointZeroCrossing finds the timeline coordinate nearest to cut at
// which both clips have a zero-crossing edge. Candidate distance grows
// outwards, so the nearest joint crossing wins.
func FindJointZeroCrossing(left, right *Clip, cut int64, maxWindow int64) (int64, bool) {
	if left == nil || right == nil {
		return 0, false
	}
	for d := int64(0); d <= maxWindow; d++ {
		for _, cand := range candidatesAtDistance(cut, d) {
			if clipHasCrossingNear(left, cand) && clipHasCrossingNear(right, cand) {
				return cand, true
			}
		}
	}
	return 0, false
}

// EditZeroCrossingReport describes zero-crossing proximity of both clip
// sides at the shared cut. Distances are in samples.
type EditZeroCrossingReport struct {
	Cut int64

	LeftFound      bool
	LeftEdgeSample int64
	LeftDistance   int64

	RightFound      bool
	RightEdgeSample int64
	RightDistance   int64

	// OnCut is true when both sides have a crossing edge exactly at Cut.
	OnCut bool
}

// CheckEditZeroCrossings measures each side's nearest crossing to cut,
// searching window samples in each direction.
func CheckEditZeroCrossings(left, right *Clip, cut, window int64) EditZeroCrossingReport {
	rep := EditZeroCrossingReport{Cut: cut}
	if edge, dist, ok := nearestTimelineZeroCrossing(left, cut, window); ok {
		rep.LeftFound = true
		rep.LeftEdgeSample = edge
		rep.LeftDistance = dist
	}
	if edge, dist, ok := nearestTimelineZeroCrossing(right, cut, window); ok {
		rep.RightFound = true
		rep.RightEdgeSample = edge
		rep.RightDistance = dist
	}
	rep.OnCut = rep.LeftFound && rep.RightFound &&
		rep.LeftDistance == 0 && rep.RightDistance == 0
	return rep
}

func candidatesAtDistance(center, d int64) []int64 {
	if d == 0 {
		return []int64{center}
	}
	return []int64{center - d, center + d}
}

// clipHasCrossingNear reports whether a crossing edge sits exactly at the
// timeline coordinate cand (edge whose right-hand sample maps to cand).
func clipHasCrossingNear(c *Clip, cand int64) bool {
	idx := c.sourceIndex(cand)
	if idx < 1 || int(idx) >= len(c.Samples) {
		return false
	}
	return isCrossingEdge(c.Samples[idx-1], c.Samples[idx])
}
