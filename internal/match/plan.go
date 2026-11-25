package match

import (
	"fmt"
	"path/filepath"
	"sort"

	"mediarelink/internal/media"
)

// Decision thresholds. Anything below autoConfidence is blocked: the
// relink must be confirmed by a human before it may be applied.
const (
	autoConfidence = 0.85
	autoMargin     = 0.10
	reviewFloor    = 0.45
)

// Status is the per-asset verdict.
type Status string

const (
	StatusOK       Status = "ok"       // file still present at recorded path
	StatusRelinked Status = "relinked" // confidently matched to a new path
	StatusBlocked  Status = "blocked"  // low confidence: must not auto-apply
)

// Block reasons.
const (
	ReasonNoCandidate   = "no_candidate"
	ReasonLowConfidence = "low_confidence"
	ReasonAmbiguous     = "ambiguous_candidates"
	ReasonConflict      = "path_claimed_by_multiple_assets"
)

// Candidate is one scored file for an asset.
type Candidate struct {
	Path         string     `json:"path"`
	Confidence   float64    `json:"confidence"`
	Evidence     []Evidence `json:"evidence"`
	Rejected     bool       `json:"rejected"`
	RejectReason string     `json:"reject_reason,omitempty"`
}

// AssetPlan is the decision for one project asset.
type AssetPlan struct {
	MediaID    string      `json:"media_id"`
	Name       string      `json:"name"`
	OldPath    string      `json:"old_path"`
	Status     Status      `json:"status"`
	NewPath    string      `json:"new_path,omitempty"`
	Confidence float64     `json:"confidence,omitempty"`
	Reason     string      `json:"reason,omitempty"`
	Effects    []string    `json:"effects,omitempty"`
	Candidates []Candidate `json:"candidates,omitempty"`
}

// Plan scores every missing asset against every probed file and decides
// per asset. A file may serve at most one asset; competing claims block
// both sides. Decoys (same name, different content) are rejected on
// content evidence and kept in the report as proof.
func Plan(missing []Expected, probes []media.Probe) []AssetPlan {
	plans := make([]AssetPlan, 0, len(missing))
	claims := map[string][]string{} // candidate path -> media IDs

	for _, exp := range missing {
		ap := AssetPlan{
			MediaID: exp.MediaID,
			Name:    exp.Name,
			OldPath: exp.OldPath,
			Status:  StatusBlocked,
			Reason:  ReasonNoCandidate,
		}

		scored := make([]Candidate, 0, len(probes))
		for _, p := range probes {
			conf, ev := Score(exp, p)
			c := Candidate{Path: p.Path, Confidence: conf, Evidence: ev}
			if IsDecoy(ev) {
				c.Rejected = true
				c.RejectReason = "content differs (frame hash and audio fingerprint mismatch)"
			}
			scored = append(scored, c)
		}
		sort.Slice(scored, func(i, j int) bool {
			if scored[i].Confidence != scored[j].Confidence {
				return scored[i].Confidence > scored[j].Confidence
			}
			return scored[i].Path < scored[j].Path
		})

		// Keep the report focused: the best candidates plus rejected
		// decoys that carry the recorded file name (the actual traps).
		n := min(len(scored), 5)
		kept := make([]Candidate, 0, n+2)
		kept = append(kept, scored[:n]...)
		for _, c := range scored[n:] {
			if c.Rejected && filepath.Base(c.Path) == filepath.Base(exp.OldPath) {
				kept = append(kept, c)
			}
		}
		ap.Candidates = kept

		var best, second *Candidate
		for i := range scored {
			if scored[i].Rejected {
				continue
			}
			if best == nil {
				best = &scored[i]
			} else {
				second = &scored[i]
				break
			}
		}

		switch {
		case best == nil || best.Confidence < reviewFloor:
			ap.Reason = ReasonNoCandidate
		case best.Confidence < autoConfidence:
			ap.Reason = ReasonLowConfidence
		case second != nil && best.Confidence-second.Confidence < autoMargin:
			ap.Reason = ReasonAmbiguous
		default:
			ap.Status = StatusRelinked
			ap.NewPath = best.Path
			ap.Confidence = best.Confidence
			ap.Reason = ""
			claims[best.Path] = append(claims[best.Path], exp.MediaID)
		}
		plans = append(plans, ap)
	}

	// One file cannot back two assets: block every conflicting claim.
	for path, ids := range claims {
		if len(ids) < 2 {
			continue
		}
		for i := range plans {
			if plans[i].NewPath == path {
				plans[i].Status = StatusBlocked
				plans[i].NewPath = ""
				plans[i].Confidence = 0
				plans[i].Reason = ReasonConflict
			}
		}
	}
	return plans
}

// Summary aggregates verdicts for reporting.
type Summary struct {
	Total    int `json:"total"`
	OK       int `json:"ok"`
	Missing  int `json:"missing"`
	Relinked int `json:"relinked"`
	Blocked  int `json:"blocked"`
}

// Summarize folds asset plans into counters. okCount covers assets whose
// recorded path still resolves and which never entered the matcher.
func Summarize(plans []AssetPlan, okCount int) Summary {
	s := Summary{OK: okCount}
	for _, p := range plans {
		switch p.Status {
		case StatusRelinked:
			s.Relinked++
		case StatusBlocked:
			s.Blocked++
		}
	}
	s.Missing = s.Relinked + s.Blocked
	s.Total = s.Missing + s.OK
	return s
}

// String renders a one-line verdict for logs.
func (p AssetPlan) String() string {
	if p.Status == StatusRelinked {
		return fmt.Sprintf("%s -> %s (confidence %.3f)", p.MediaID, p.NewPath, p.Confidence)
	}
	return fmt.Sprintf("%s BLOCKED (%s)", p.MediaID, p.Reason)
}
