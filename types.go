package clipmerge

import "fmt"

// Property names managed by the merge engine, in canonical order.
const (
	PropertyInPoint  = "inPoint"
	PropertyOutPoint = "outPoint"
	PropertySpeed    = "speed"
)

// PropertyOrder is the canonical, deterministic property order used
// everywhere: reports, checks, and hashing never depend on map iteration.
var PropertyOrder = []string{PropertyInPoint, PropertyOutPoint, PropertySpeed}

// Speed is a playback-rate rational: timelineDuration = sourceSpan / Speed.
// Numerator and Denominator are always kept normalized (gcd-reduced, Denominator > 0).
type Speed struct {
	Numerator   int64 `json:"numerator"`
	Denominator int64 `json:"denominator"`
}

// Normalized returns the gcd-reduced form with a positive denominator.
func (s Speed) Normalized() Speed {
	if s.Denominator < 0 {
		s.Numerator, s.Denominator = -s.Numerator, -s.Denominator
	}
	g := gcd(abs(s.Numerator), s.Denominator)
	if g > 1 {
		s.Numerator /= g
		s.Denominator /= g
	}
	return s
}

// String renders the speed deterministically: "5" or "3/2".
func (s Speed) String() string {
	s = s.Normalized()
	if s.Denominator == 1 {
		return fmt.Sprintf("%d", s.Numerator)
	}
	return fmt.Sprintf("%d/%d", s.Numerator, s.Denominator)
}

// Source describes the media a clip refers to: the hard media boundary.
type Source struct {
	ID                  string `json:"id"`
	Duration            int64  `json:"duration"`
	MinTimelineDuration int64  `json:"minTimelineDuration"`
}

// Clip is a source-range + playback-rate placement on the timeline.
type Clip struct {
	ID       string `json:"id"`
	SourceID string `json:"sourceId"`
	Revision int64  `json:"revision"`
	InPoint  int64  `json:"inPoint"`
	OutPoint int64  `json:"outPoint"`
	Speed    Speed  `json:"speed"`
}

// SourceSpan is the amount of source media the clip consumes: OutPoint - InPoint.
func (c Clip) SourceSpan() int64 { return c.OutPoint - c.InPoint }

// TimelineDuration is the exact rational timeline occupancy: SourceSpan / Speed.
func (c Clip) TimelineDuration() Speed {
	s := Speed{Numerator: c.SourceSpan() * c.Speed.Denominator, Denominator: c.Speed.Numerator}
	return s.Normalized()
}

// Edit is one property-level operation from one client against a base revision.
// It carries no timestamp: merge semantics never depend on arrival order.
type Edit struct {
	ID           string      `json:"id"`
	ClientID     string      `json:"clientId"`
	BaseRevision int64       `json:"baseRevision"`
	Property     string      `json:"property"`
	Value        interface{} `json:"value"`
}

// MergeRequest is the deterministic input to the merge engine.
type MergeRequest struct {
	Source Source `json:"source"`
	Base   Clip   `json:"base"`
	Edits  []Edit `json:"edits"`
}

func gcd(a, b int64) int64 {
	for b != 0 {
		a, b = b, a%b
	}
	if a < 0 {
		return -a
	}
	return a
}

func abs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
