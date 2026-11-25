package safereflow

import (
	"fmt"
	"sort"
)

// DefaultConfidenceThreshold is the gate used when the document or caller
// does not provide one. Tracking at or above 0.8 is considered trustworthy
// enough to move subtitles automatically.
const DefaultConfidenceThreshold = 0.8

// SafeTrack is a validated, pixel-coordinate, time-sorted dynamic safe area.
type SafeTrack struct {
	Canvas    Canvas
	Threshold float64
	spans     []SafeSpan
}

// Spans returns a copy of the ordered safe-area intervals.
func (t *SafeTrack) Spans() []SafeSpan {
	out := make([]SafeSpan, len(t.spans))
	copy(out, t.spans)
	return out
}

// NewSafeTrack validates the document, converts normalized coordinates and
// sorts intervals by start time. The input document is not modified.
func NewSafeTrack(doc SafeDocument) (*SafeTrack, error) {
	if doc.Canvas.Width <= 0 || doc.Canvas.Height <= 0 {
		return nil, fmt.Errorf("safereflow: canvas width/height must be positive, got %vx%v",
			doc.Canvas.Width, doc.Canvas.Height)
	}
	unit := doc.Unit
	if unit == "" {
		unit = "pixel"
	}
	if unit != "pixel" && unit != "normalized" {
		return nil, fmt.Errorf("safereflow: unsupported unit %q (want pixel|normalized)", doc.Unit)
	}

	threshold := DefaultConfidenceThreshold
	if doc.ConfidenceThreshold != nil {
		threshold = *doc.ConfidenceThreshold
	}
	if threshold < 0 || threshold > 1 {
		return nil, fmt.Errorf("safereflow: confidence_threshold must be in [0,1], got %v", threshold)
	}

	spans := make([]SafeSpan, 0, len(doc.SafeAreas))
	for i, sp := range doc.SafeAreas {
		if sp.End <= sp.Start {
			return nil, fmt.Errorf("safereflow: safe_areas[%d] end must be greater than start", i)
		}
		if sp.Start < -eps {
			return nil, fmt.Errorf("safereflow: safe_areas[%d] start must not be negative", i)
		}
		if sp.Confidence != nil && (*sp.Confidence < 0 || *sp.Confidence > 1) {
			return nil, fmt.Errorf("safereflow: safe_areas[%d] confidence must be in [0,1]", i)
		}
		r := sp.Rect
		if unit == "normalized" {
			r = Rect{
				X: r.X * doc.Canvas.Width,
				Y: r.Y * doc.Canvas.Height,
				W: r.W * doc.Canvas.Width,
				H: r.H * doc.Canvas.Height,
			}
		}
		if r.W <= 0 || r.H <= 0 {
			return nil, fmt.Errorf("safereflow: safe_areas[%d] rect width/height must be positive", i)
		}
		if r.X < -eps || r.Y < -eps || r.Right() > doc.Canvas.Width+eps || r.Bottom() > doc.Canvas.Height+eps {
			return nil, fmt.Errorf("safereflow: safe_areas[%d] rect %+v outside canvas %vx%v",
				i, r, doc.Canvas.Width, doc.Canvas.Height)
		}
		sp.Rect = r
		if sp.ID == "" {
			sp.ID = fmt.Sprintf("sa-%d", i+1)
		}
		spans = append(spans, sp)
	}

	sort.SliceStable(spans, func(i, j int) bool {
		if spans[i].Start != spans[j].Start {
			return spans[i].Start < spans[j].Start
		}
		return spans[i].End < spans[j].End
	})

	return &SafeTrack{Canvas: doc.Canvas, Threshold: threshold, spans: spans}, nil
}

// Slice is an atomic piece of a cue timeline after cutting at every safe-area
// boundary. At most one effective rectangle applies to a slice: overlapping
// source spans are combined by intersection (the conservative choice).
type Slice struct {
	TimeRange
	SpanIDs         []string `json:"span_ids"`
	Covered         bool     `json:"covered"`
	Confidence      float64  `json:"confidence"`
	ConfidenceKnown bool     `json:"confidence_known"`
	Rect            Rect     `json:"rect"`
}

// Trusted reports whether the slice is fully covered by tracking whose
// confidence meets threshold.
func (s Slice) Trusted(threshold float64) bool {
	return s.Covered && s.ConfidenceKnown && s.Confidence+eps >= threshold
}

// SlicesOver partitions iv at every span boundary and resolves the effective
// safe rectangle for each piece. Gaps with no span are returned as uncovered
// slices so callers never mistake "unknown" for "safe".
func (t *SafeTrack) SlicesOver(iv TimeRange) []Slice {
	// Collect cut points inside the cue interval.
	cuts := []float64{iv.Start, iv.End}
	for _, sp := range t.spans {
		if !sp.TimeRange.Overlaps(iv) {
			continue
		}
		if sp.Start > iv.Start+eps && sp.Start < iv.End-eps {
			cuts = append(cuts, sp.Start)
		}
		if sp.End > iv.Start+eps && sp.End < iv.End-eps {
			cuts = append(cuts, sp.End)
		}
	}
	sort.Float64s(cuts)

	// De-duplicate nearly-equal cuts.
	unique := cuts[:0]
	for _, c := range cuts {
		if len(unique) == 0 || c-unique[len(unique)-1] > eps {
			unique = append(unique, c)
		}
	}

	slices := make([]Slice, 0, len(unique)-1)
	for i := 0; i+1 < len(unique); i++ {
		part := TimeRange{Start: unique[i], End: unique[i+1]}
		if part.Duration() <= eps {
			continue
		}
		mid := (part.Start + part.End) / 2
		var active []SafeSpan
		for _, sp := range t.spans {
			if sp.Contains(mid) {
				active = append(active, sp)
			}
		}
		sl := Slice{TimeRange: part}
		if len(active) == 0 {
			slices = append(slices, sl)
			continue
		}
		sl.Covered = true
		sl.Confidence = 1
		sl.ConfidenceKnown = true
		rects := make([]Rect, 0, len(active))
		for _, sp := range active {
			sl.SpanIDs = append(sl.SpanIDs, sp.ID)
			rects = append(rects, sp.Rect)
			if sp.Confidence == nil {
				sl.ConfidenceKnown = false
				sl.Confidence = 0
			} else if *sp.Confidence < sl.Confidence {
				sl.Confidence = *sp.Confidence
			}
		}
		if r, ok := rectIntersectionAll(rects); ok {
			sl.Rect = r
		}
		slices = append(slices, sl)
	}
	return slices
}

// CommonRectOver returns the rectangle that is safe across the whole interval
// (intersection of every covered slice's effective rect), the minimum
// confidence and whether the entire interval is covered by known tracking.
func (t *SafeTrack) CommonRectOver(iv TimeRange) (common Rect, minConf float64, known, fullCoverage, anyRect bool) {
	slices := t.SlicesOver(iv)
	minConf = 1
	known = true
	fullCoverage = true
	var rects []Rect
	for _, sl := range slices {
		if !sl.Covered {
			fullCoverage = false
			known = false
			continue
		}
		if !sl.ConfidenceKnown {
			known = false
		}
		if sl.Confidence < minConf {
			minConf = sl.Confidence
		}
		if !sl.Rect.Empty() {
			rects = append(rects, sl.Rect)
		}
	}
	if len(rects) > 0 {
		common, anyRect = rectIntersectionAll(rects)
	}
	if !known {
		minConf = 0
	}
	return common, minConf, known, fullCoverage, anyRect
}
