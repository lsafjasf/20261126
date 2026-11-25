package safereflow

// Status is the per-cue verdict after intersecting with the dynamic track.
type Status string

const (
	// StatusSafe: full coverage, no geometric overflow.
	StatusSafe Status = "safe"
	// StatusViolation: a trustworthy slice shows the cue outside safe area.
	StatusViolation Status = "violation"
	// StatusSuspected: overflow only seen on low/unknown-confidence slices.
	StatusSuspected Status = "suspected"
	// StatusIndeterminate: coverage gaps prevent a definitive verdict.
	StatusIndeterminate Status = "indeterminate"
)

// Gate summarizes whether the tracking data governing a cue is trustworthy
// enough for automatic edits.
type Gate struct {
	Threshold        float64  `json:"threshold"`
	MinConfidence    float64  `json:"min_confidence"`
	ConfidenceKnown  bool     `json:"confidence_known"`
	FullCoverage     bool     `json:"full_coverage"`
	Trusted          bool     `json:"trusted"`
	AutoAllowed      bool     `json:"auto_allowed"`
	MigrationAllowed bool     `json:"migration_allowed"`
	BlockReasons     []string `json:"block_reasons,omitempty"`
}

func buildGate(threshold, minConf float64, known, fullCoverage bool) Gate {
	g := Gate{
		Threshold:       threshold,
		MinConfidence:   minConf,
		ConfidenceKnown: known,
		FullCoverage:    fullCoverage,
	}
	if !fullCoverage {
		g.BlockReasons = append(g.BlockReasons, "coverage_gap")
	} else if !known {
		g.BlockReasons = append(g.BlockReasons, "confidence_unknown")
	} else if minConf+eps < threshold {
		g.BlockReasons = append(g.BlockReasons, "low_confidence")
	}
	g.Trusted = fullCoverage && known && minConf+eps >= threshold
	// Default strict policy: automatic edits require a fully trusted track.
	g.AutoAllowed = g.Trusted
	// Moving a cue is the riskiest action and can never bypass the gate.
	g.MigrationAllowed = g.Trusted
	return g
}

// SegmentFinding is the cue-vs-safe intersection for one atomic time slice.
type SegmentFinding struct {
	TimeRange
	SpanIDs           []string `json:"span_ids,omitempty"`
	Covered           bool     `json:"covered"`
	Trusted           bool     `json:"trusted"`
	Confidence        float64  `json:"confidence"`
	ConfidenceKnown   bool     `json:"confidence_known"`
	SafeRect          Rect     `json:"safe_rect"`
	Intersection      Rect     `json:"intersection"`
	Overflow          Overflow `json:"overflow"`
	IntersectionRatio float64  `json:"intersection_ratio"`
	Inside            bool     `json:"inside"`
}

// Finding is the complete analysis of one cue.
type Finding struct {
	CueID              string           `json:"cue_id"`
	TimeRange          TimeRange        `json:"time"`
	Box                Rect             `json:"box"`
	Status             Status           `json:"status"`
	GeometricViolation bool             `json:"geometric_violation"`
	Gate               Gate             `json:"gate"`
	Segments           []SegmentFinding `json:"segments"`
	CommonRect         *Rect            `json:"common_safe_rect,omitempty"`
	Plans              []*Plan          `json:"plans,omitempty"`
	RecommendedPlanID  string           `json:"recommended_plan_id,omitempty"`
}

// ReportSummary counts verdicts and plans.
type ReportSummary struct {
	Cues              int `json:"cues"`
	Safe              int `json:"safe"`
	Violations        int `json:"violations"`
	Suspected         int `json:"suspected"`
	Indeterminate     int `json:"indeterminate"`
	WithAutoPlan      int `json:"with_auto_plan"`
	MigrationsBlocked int `json:"migrations_blocked_low_confidence"`
}

// Report is the full analysis output.
type Report struct {
	Canvas              Canvas        `json:"canvas"`
	ConfidenceThreshold float64       `json:"confidence_threshold"`
	Summary             ReportSummary `json:"summary"`
	Findings            []*Finding    `json:"findings"`
}

// Engine binds a cue document to a dynamic safe track and produces reports.
type Engine struct {
	Doc       *CueDocument
	Track     *SafeTrack
	Measurer  Measurer
	Threshold float64 // <=0 uses Track.Threshold

	// AllowUntrustedShrink permits automatic rewrap/shorten on untrusted
	// tracking. It never affects the hard migration confidence gate.
	AllowUntrustedShrink bool

	// PixelAlign rounds proposed boxes to integer pixels.
	PixelAlign bool

	resolved []ResolvedCue
}

// NewEngine builds an engine for doc and track.
func NewEngine(doc *CueDocument, track *SafeTrack) *Engine {
	return &Engine{Doc: doc, Track: track, Measurer: NewGridMeasurer()}
}

// AnalyzeDocument is a one-shot convenience: build a default Engine and run
// the dynamic safe-area analysis with plans.
func AnalyzeDocument(doc *CueDocument, track *SafeTrack) (*Report, error) {
	return NewEngine(doc, track).Analyze()
}

func (e *Engine) threshold() float64 {
	if e.Threshold > 0 && e.Threshold <= 1 {
		return e.Threshold
	}
	if e.Track != nil && e.Track.Threshold > 0 {
		return e.Track.Threshold
	}
	return DefaultConfidenceThreshold
}

// Analyze intersects every cue with the dynamic track and builds plans.
func (e *Engine) Analyze() (*Report, error) {
	if e.Doc == nil {
		return nil, ErrNilDocument
	}
	if e.Track == nil {
		return nil, ErrNilTrack
	}
	threshold := e.threshold()
	rep := &Report{
		Canvas:              e.Doc.Canvas,
		ConfidenceThreshold: threshold,
	}

	e.resolved = make([]ResolvedCue, 0, len(e.Doc.Cues))
	for _, c := range e.Doc.Cues {
		if c == nil {
			continue
		}
		rc := ResolveCue(e.Doc, c, e.Measurer)
		e.resolved = append(e.resolved, rc)
		f := e.analyzeCue(rc, threshold)
		e.buildPlans(rc, f)
		rep.Findings = append(rep.Findings, f)
	}

	rep.Summary.Cues = len(rep.Findings)
	for _, f := range rep.Findings {
		switch f.Status {
		case StatusSafe:
			rep.Summary.Safe++
		case StatusViolation:
			rep.Summary.Violations++
		case StatusSuspected:
			rep.Summary.Suspected++
		case StatusIndeterminate:
			rep.Summary.Indeterminate++
		}
		if f.RecommendedPlanID != "" {
			for _, p := range f.Plans {
				if p.ID == f.RecommendedPlanID && p.AutoEligible {
					rep.Summary.WithAutoPlan++
				}
			}
		}
		for _, p := range f.Plans {
			if p.Action == ActionMigrate && p.Feasible && !p.Gate.MigrationAllowed {
				rep.Summary.MigrationsBlocked++
				break
			}
		}
	}
	return rep, nil
}

func (e *Engine) analyzeCue(rc ResolvedCue, threshold float64) *Finding {
	c := rc.Cue
	f := &Finding{
		CueID:     c.ID,
		TimeRange: c.TimeRange,
		Box:       rc.Box,
	}

	slices := e.Track.SlicesOver(c.TimeRange)
	var trustedViolation, geomViolation bool

	for _, sl := range slices {
		sf := SegmentFinding{
			TimeRange:       sl.TimeRange,
			SpanIDs:         sl.SpanIDs,
			Covered:         sl.Covered,
			Trusted:         sl.Trusted(threshold),
			Confidence:      sl.Confidence,
			ConfidenceKnown: sl.ConfidenceKnown,
			SafeRect:        sl.Rect,
		}
		if sl.Covered && !sl.Rect.Empty() {
			sf.Intersection = rc.Box.Intersect(sl.Rect)
			sf.Overflow = OverflowOf(rc.Box, sl.Rect)
			sf.Inside = !sf.Overflow.Any()
			if rc.Box.Area() > 0 {
				sf.IntersectionRatio = sf.Intersection.Area() / rc.Box.Area()
			}
			if !sf.Inside {
				geomViolation = true
				if sf.Trusted {
					trustedViolation = true
				}
			}
		}
		f.Segments = append(f.Segments, sf)
	}

	common, minConf, known, fullCoverage, hasRect := e.Track.CommonRectOver(c.TimeRange)
	f.Gate = buildGate(threshold, minConf, known, fullCoverage)
	if hasRect && !common.Empty() {
		// Copy to take a stable address.
		cr := common
		f.CommonRect = &cr
	}

	f.GeometricViolation = geomViolation
	switch {
	case trustedViolation:
		f.Status = StatusViolation
	case geomViolation:
		f.Status = StatusSuspected
	case !fullCoverage:
		f.Status = StatusIndeterminate
	default:
		f.Status = StatusSafe
	}
	return f
}
