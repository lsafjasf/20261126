package safereflow

import "fmt"

// Action identifies the remediation kind.
type Action string

const (
	// ActionRewrap 拆行: re-wrap text onto more/shorter lines.
	ActionRewrap Action = "rewrap"
	// ActionShorten 缩短: condense/truncate text with an ellipsis.
	ActionShorten Action = "shorten"
	// ActionMigrate 迁移: translate the unchanged cue box into the safe area.
	ActionMigrate Action = "migrate"
	// ActionTimeSplit 分段迁移: split the cue at safe-area boundaries and
	// move each sub-cue independently.
	ActionTimeSplit Action = "time_split"
)

// Move describes a translation applied to a cue box.
type Move struct {
	DX float64 `json:"dx"`
	DY float64 `json:"dy"`
}

// TimedBox is a box target valid for one sub-interval (time-split plan).
type TimedBox struct {
	TimeRange
	Rect Rect    `json:"rect"`
	DX   float64 `json:"dx"`
	DY   float64 `json:"dy"`
}

// Plan is one concrete remediation candidate.
type Plan struct {
	ID           string   `json:"id"`
	CueID        string   `json:"cue_id"`
	Action       Action   `json:"action"`
	Feasible     bool     `json:"feasible"`
	AutoEligible bool     `json:"auto_eligible"`
	Recommended  bool     `json:"recommended,omitempty"`
	Confidence   float64  `json:"confidence"`
	Summary      string   `json:"summary"`
	Reasons      []string `json:"reasons,omitempty"`
	Gate         Gate     `json:"gate"`

	// Text/Lines are set for text-changing plans.
	Text  string   `json:"text,omitempty"`
	Lines []string `json:"lines,omitempty"`

	// Box is the proposed absolute cue box after the edit.
	Box *Rect `json:"box,omitempty"`
	// Move is set when Box differs from the current box by translation.
	Move *Move `json:"move,omitempty"`

	// ContentLoss marks shortening plans that drop words/characters.
	ContentLoss bool `json:"content_loss,omitempty"`

	// Segments is set for ActionTimeSplit.
	Segments []TimedBox `json:"segments,omitempty"`
}

// ChangesText reports whether applying the plan replaces cue text.
func (p *Plan) ChangesText() bool { return p != nil && p.Text != "" }

// ChangesPosition reports whether applying the plan moves/resizes the box.
func (p *Plan) ChangesPosition() bool {
	return p != nil && (p.Move != nil || p.Action == ActionMigrate || p.Action == ActionTimeSplit)
}

func (e *Engine) newPlan(f *Finding, action Action) *Plan {
	return &Plan{
		ID:         fmt.Sprintf("%s-%s", f.CueID, action),
		CueID:      f.CueID,
		Action:     action,
		Gate:       f.Gate,
		Confidence: f.Gate.MinConfidence,
	}
}

// finalizeBox rounds (optionally) and attaches box/move information.
func (e *Engine) finalizeBox(p *Plan, rc ResolvedCue, box Rect, moved bool) {
	if e.PixelAlign {
		box = box.RoundToPixel()
	}
	b := box
	p.Box = &b
	if moved {
		p.Move = &Move{DX: box.X - rc.Box.X, DY: box.Y - rc.Box.Y}
		if approxEqual(p.Move.DX, 0) && approxEqual(p.Move.DY, 0) {
			p.Move = nil
		}
	}
}

// markAuto sets AutoEligible according to the confidence gate and policy.
// Position changes always require MigrationAllowed; pure shrink fixes may be
// relaxed via Engine.AllowUntrustedShrink (but are still never recommended as
// automatic moves).
func (e *Engine) markAuto(p *Plan, involvesMove bool) {
	if !p.Feasible {
		p.AutoEligible = false
		return
	}
	if involvesMove {
		p.AutoEligible = p.Gate.MigrationAllowed
		if !p.Gate.MigrationAllowed {
			p.Reasons = appendUnique(p.Reasons, "not_auto:migration_requires_trusted_safe_area")
		}
		return
	}
	if p.Gate.AutoAllowed || e.AllowUntrustedShrink {
		p.AutoEligible = true
	} else {
		p.AutoEligible = false
		p.Reasons = appendUnique(p.Reasons, "not_auto:untrusted_safe_area_needs_review")
	}
}

func appendUnique(xs []string, x string) []string {
	for _, v := range xs {
		if v == x {
			return xs
		}
	}
	return append(xs, x)
}

// buildPlans generates every candidate for the cue and picks a recommendation.
func (e *Engine) buildPlans(rc ResolvedCue, f *Finding) {
	// Safe cues need no remediation.
	if f.Status == StatusSafe {
		return
	}

	rw := e.buildRewrap(rc, f)
	sh := e.buildShorten(rc, f)
	mv := e.buildMigrate(rc, f)
	ts := e.buildTimeSplit(rc, f)

	for _, p := range []*Plan{rw, sh, mv, ts} {
		if p != nil {
			f.Plans = append(f.Plans, p)
		}
	}

	// Recommendation order: lossless rewrap, then migration (no content loss),
	// then shortening, then time-split. Only feasible plans are considered.
	order := []Action{ActionRewrap, ActionMigrate, ActionShorten, ActionTimeSplit}
	byAction := map[Action]*Plan{}
	for _, p := range f.Plans {
		byAction[p.Action] = p
	}
	for _, a := range order {
		p := byAction[a]
		if p == nil || !p.Feasible {
			continue
		}
		p.Recommended = true
		f.RecommendedPlanID = p.ID
		return
	}
}

// candidateFitsCommon reports whether box lies in the common safe rect.
func candidateFitsCommon(f *Finding, box Rect) bool {
	if f.CommonRect == nil || f.CommonRect.Empty() {
		return false
	}
	return FitsInside(box, *f.CommonRect)
}
