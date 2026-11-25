package safereflow

import (
	"errors"
	"fmt"
)

// ApplyOptions controls plan application.
type ApplyOptions struct {
	// PlanIDs selects exact plans. When empty, Apply uses each finding's
	// recommended plan but only if it is AutoEligible (the automatic path).
	PlanIDs []string

	// AcknowledgeLowConfidenceMove is a human-review escape hatch: an
	// operator may explicitly approve a migration whose tracking confidence
	// is below threshold. It is never set by the automatic path and every
	// such application is recorded with mode manual_low_confidence_ack.
	AcknowledgeLowConfidenceMove bool
}

// AppliedChange records one applied plan.
type AppliedChange struct {
	CueID   string `json:"cue_id"`
	PlanID  string `json:"plan_id"`
	Action  Action `json:"action"`
	Mode    string `json:"mode"`
	Summary string `json:"summary"`
}

// BlockedChange records a plan that was not applied.
type BlockedChange struct {
	CueID  string `json:"cue_id"`
	PlanID string `json:"plan_id"`
	Reason string `json:"reason"`
}

// ApplyReport lists applied and blocked plans.
type ApplyReport struct {
	Applied []AppliedChange `json:"applied"`
	Blocked []BlockedChange `json:"blocked,omitempty"`
}

// HasApplied reports whether at least one change was applied.
func (r *ApplyReport) HasApplied() bool { return r != nil && len(r.Applied) > 0 }

// Apply executes plans against a deep copy of the engine document; the
// engine's source document is never mutated.
//
// With no PlanIDs it runs the automatic path: only recommended, AutoEligible
// plans are applied. Low-confidence migrations are therefore never executed
// automatically. With explicit PlanIDs the named feasible plans are applied,
// but a migration blocked by the confidence gate still returns
// ErrLowConfidence unless AcknowledgeLowConfidenceMove is set.
func (e *Engine) Apply(rep *Report, opts ApplyOptions) (*CueDocument, *ApplyReport, error) {
	if e.Doc == nil {
		return nil, nil, ErrNilDocument
	}
	if rep == nil {
		return nil, nil, errors.New("safereflow: nil report")
	}

	planByID := map[string]*Plan{}
	findingsByCue := map[string]*Finding{}
	for _, f := range rep.Findings {
		findingsByCue[f.CueID] = f
		for _, p := range f.Plans {
			planByID[p.ID] = p
		}
	}

	ar := &ApplyReport{Applied: []AppliedChange{}, Blocked: []BlockedChange{}}

	type selected struct {
		plan *Plan
		mode string
	}
	byCue := map[string]selected{}

	if len(opts.PlanIDs) == 0 {
		// Automatic path.
		for _, f := range rep.Findings {
			if f.RecommendedPlanID == "" {
				continue
			}
			p := planByID[f.RecommendedPlanID]
			if p == nil {
				continue
			}
			if !p.Feasible || !p.AutoEligible {
				reason := "not_auto_eligible"
				if p != nil && planMovesBox(p) && !p.Gate.MigrationAllowed {
					reason = "low_confidence_move_not_automatic"
				}
				ar.Blocked = append(ar.Blocked, BlockedChange{
					CueID: f.CueID, PlanID: f.RecommendedPlanID, Reason: reason,
				})
				continue
			}
			if _, dup := byCue[f.CueID]; dup {
				continue
			}
			byCue[f.CueID] = selected{plan: p, mode: "auto"}
		}
	} else {
		// Explicit selection path: validate everything before mutating.
		var hardErr error
		for _, id := range opts.PlanIDs {
			p, ok := planByID[id]
			if !ok {
				hardErr = errors.Join(hardErr, fmt.Errorf("%w: %s", ErrPlanNotFound, id))
				continue
			}
			if !p.Feasible {
				hardErr = errors.Join(hardErr, fmt.Errorf("%w: %s", ErrPlanBlocked, id))
				continue
			}
			if planMovesBox(p) && !p.Gate.MigrationAllowed && !opts.AcknowledgeLowConfidenceMove {
				hardErr = errors.Join(hardErr,
					fmt.Errorf("%w: plan %s confidence=%.2f threshold=%.2f coverage=%v",
						ErrLowConfidence, id, p.Gate.MinConfidence, p.Gate.Threshold, p.Gate.FullCoverage))
				continue
			}
			if prev, dup := byCue[p.CueID]; dup && prev.plan.ID != p.ID {
				hardErr = errors.Join(hardErr,
					fmt.Errorf("%w: cue %s has multiple selected plans (%s, %s)",
						ErrPlanBlocked, p.CueID, prev.plan.ID, p.ID))
				continue
			}
			mode := "selected"
			if planMovesBox(p) && !p.Gate.MigrationAllowed && opts.AcknowledgeLowConfidenceMove {
				mode = "manual_low_confidence_ack"
			}
			byCue[p.CueID] = selected{plan: p, mode: mode}
		}
		if hardErr != nil {
			// Atomic failure: return the untouched source document.
			return e.Doc, ar, hardErr
		}
	}

	doc := e.Doc.Clone()
	out := make([]*Cue, 0, len(doc.Cues))

	for _, c := range doc.Cues {
		sel, ok := byCue[c.ID]
		if !ok {
			out = append(out, c)
			continue
		}
		p := sel.plan

		if p.Action == ActionTimeSplit && len(p.Segments) > 0 {
			for i, seg := range p.Segments {
				sub := c.Clone()
				sub.ID = fmt.Sprintf("%s#%d", c.ID, i+1)
				sub.TimeRange = seg.TimeRange
				r := seg.Rect
				sub.Box = &r
				out = append(out, sub)
			}
		} else {
			if p.Text != "" {
				c.Text = p.Text
			}
			if p.Box != nil {
				r := *p.Box
				c.Box = &r
			}
			out = append(out, c)
		}

		ar.Applied = append(ar.Applied, AppliedChange{
			CueID:   c.ID,
			PlanID:  p.ID,
			Action:  p.Action,
			Mode:    sel.mode,
			Summary: p.Summary,
		})
	}
	doc.Cues = out
	return doc, ar, nil
}

// ApplyAuto is shorthand for the automatic path.
func (e *Engine) ApplyAuto(rep *Report) (*CueDocument, *ApplyReport, error) {
	return e.Apply(rep, ApplyOptions{})
}

// planMovesBox reports whether applying p changes the cue's position in a way
// that counts as "moving" for the confidence gate. Pure anchor-preserving
// rewraps do not count; explicit translation and migrations do.
func planMovesBox(p *Plan) bool {
	if p == nil {
		return false
	}
	if p.Action == ActionMigrate || p.Action == ActionTimeSplit {
		return true
	}
	return p.Move != nil
}

// Clone returns a deep copy of the document.
func (d *CueDocument) Clone() *CueDocument {
	if d == nil {
		return nil
	}
	cp := *d
	cp.Cues = make([]*Cue, len(d.Cues))
	for i, c := range d.Cues {
		cp.Cues[i] = c.Clone()
	}
	return &cp
}

// Clone returns a deep copy of the cue.
func (c *Cue) Clone() *Cue {
	if c == nil {
		return nil
	}
	cp := *c
	if c.Box != nil {
		b := *c.Box
		cp.Box = &b
	}
	if c.Style != nil {
		s := *c.Style
		cp.Style = &s
	}
	return &cp
}
