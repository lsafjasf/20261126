package safereflow

import (
	"errors"
	"testing"
)

func singleSafe(conf float64, rect Rect) SafeDocument {
	d := SafeDocument{
		Canvas: Canvas{Width: 1920, Height: 1080},
		SafeAreas: []SafeSpan{{
			TimeRange:  TimeRange{Start: 0, End: 10},
			Rect:       rect,
			Confidence: &conf,
		}},
	}
	return d
}

func buildEngine(t *testing.T, doc *CueDocument, safe SafeDocument) *Engine {
	t.Helper()
	track, err := NewSafeTrack(safe)
	if err != nil {
		t.Fatal(err)
	}
	return NewEngine(doc, track)
}

func findFinding(rep *Report, id string) *Finding {
	for _, f := range rep.Findings {
		if f.CueID == id {
			return f
		}
	}
	return nil
}

func findPlan(f *Finding, action Action) *Plan {
	for _, p := range f.Plans {
		if p.Action == action {
			return p
		}
	}
	return nil
}

func TestIntersectionAndHighConfidenceMigrate(t *testing.T) {
	safeRect := Rect{X: 200, Y: 200, W: 1520, H: 680} // right 1720, bottom 880
	doc := &CueDocument{
		Canvas: Canvas{Width: 1920, Height: 1080},
		Cues: []*Cue{{
			ID:        "m1",
			TimeRange: TimeRange{Start: 1, End: 4},
			Text:      "Short",
			Box:       &Rect{X: 100, Y: 820, W: 700, H: 90}, // left -100, bottom 910 -> +30
		}},
	}
	eng := buildEngine(t, doc, singleSafe(0.95, safeRect))
	rep, err := eng.Analyze()
	if err != nil {
		t.Fatal(err)
	}
	f := findFinding(rep, "m1")
	if f == nil {
		t.Fatal("missing finding")
	}
	if f.Status != StatusViolation {
		t.Fatalf("status = %s, want violation", f.Status)
	}
	if len(f.Segments) != 1 {
		t.Fatalf("segments = %d", len(f.Segments))
	}
	seg := f.Segments[0]
	if !near(seg.Overflow.Left, 100) || !near(seg.Overflow.Bottom, 30) {
		t.Fatalf("overflow = %+v", seg.Overflow)
	}
	// Intersection 600*60 = 36000 over box 700*90 = 63000 -> 4/7.
	if !near(seg.IntersectionRatio, 36000.0/63000.0) {
		t.Fatalf("ratio = %v", seg.IntersectionRatio)
	}

	mp := findPlan(f, ActionMigrate)
	if mp == nil || !mp.Feasible {
		t.Fatalf("migrate plan missing/infeasible: %+v", mp)
	}
	if !mp.AutoEligible || !mp.Gate.MigrationAllowed {
		t.Fatal("high confidence migrate must be auto eligible")
	}
	if mp.Move == nil || !near(mp.Move.DX, 100) || !near(mp.Move.DY, -30) {
		t.Fatalf("move = %+v", mp.Move)
	}
	if f.RecommendedPlanID != mp.ID {
		t.Fatalf("recommend = %s, want %s", f.RecommendedPlanID, mp.ID)
	}

	applied, ar, err := eng.ApplyAuto(rep)
	if err != nil {
		t.Fatal(err)
	}
	if len(ar.Applied) != 1 {
		t.Fatalf("applied = %+v", ar.Applied)
	}
	got := applied.Cues[0].Box
	want := Rect{X: 200, Y: 790, W: 700, H: 90}
	if got == nil || !rectNear(*got, want) {
		t.Fatalf("applied box = %+v, want %+v", got, want)
	}
	// Source document must stay untouched.
	if doc.Cues[0].Box.X != 100 {
		t.Fatal("source document was mutated")
	}

	// Re-analyzing the applied document must be clean.
	eng2 := buildEngine(t, applied, singleSafe(0.95, safeRect))
	rep2, err := eng2.Analyze()
	if err != nil {
		t.Fatal(err)
	}
	if findFinding(rep2, "m1").Status != StatusSafe {
		t.Fatalf("post-apply status = %s", findFinding(rep2, "m1").Status)
	}
}

func TestLowConfidenceNeverAutoMoves(t *testing.T) {
	safeRect := Rect{X: 200, Y: 200, W: 1520, H: 680}
	original := Rect{X: 100, Y: 820, W: 700, H: 90}
	doc := &CueDocument{
		Canvas: Canvas{Width: 1920, Height: 1080},
		Cues:   []*Cue{{ID: "low1", TimeRange: TimeRange{1, 4}, Text: "Move me maybe", Box: &original}},
	}
	eng := buildEngine(t, doc, singleSafe(0.5, safeRect))
	rep, err := eng.Analyze()
	if err != nil {
		t.Fatal(err)
	}
	f := findFinding(rep, "low1")
	if f.Status != StatusSuspected {
		t.Fatalf("status = %s, want suspected", f.Status)
	}
	if f.Gate.Trusted || f.Gate.MigrationAllowed {
		t.Fatalf("gate must be locked: %+v", f.Gate)
	}

	mp := findPlan(f, ActionMigrate)
	if mp == nil || !mp.Feasible || mp.AutoEligible {
		t.Fatalf("migrate must stay feasible-but-manual: %+v", mp)
	}

	// Automatic path changes nothing and records a block.
	autoDoc, ar, err := eng.ApplyAuto(rep)
	if err != nil {
		t.Fatal(err)
	}
	if len(ar.Applied) != 0 {
		t.Fatalf("low confidence cue must not be auto-edited: %+v", ar.Applied)
	}
	if len(ar.Blocked) != 1 || ar.Blocked[0].Reason != "low_confidence_move_not_automatic" {
		t.Fatalf("blocked = %+v", ar.Blocked)
	}
	if autoDoc.Cues[0].Box.X != 100 {
		t.Fatal("auto path moved the low-confidence cue")
	}

	// Default explicit selection is still refused by the hard gate.
	_, _, err = eng.Apply(rep, ApplyOptions{PlanIDs: []string{mp.ID}})
	if !errors.Is(err, ErrLowConfidence) {
		t.Fatalf("expected ErrLowConfidence, got %v", err)
	}
	if doc.Cues[0].Box.X != 100 {
		t.Fatal("failed apply mutated source document")
	}

	// A named human-review acknowledgement may execute, audited as manual.
	acked, ar2, err := eng.Apply(rep, ApplyOptions{
		PlanIDs:                      []string{mp.ID},
		AcknowledgeLowConfidenceMove: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(ar2.Applied) != 1 || ar2.Applied[0].Mode != "manual_low_confidence_ack" {
		t.Fatalf("unexpected apply report: %+v", ar2)
	}
	if acked.Cues[0].Box.X != 200 {
		t.Fatalf("acked box x = %v, want 200", acked.Cues[0].Box.X)
	}
}

func TestCoverageGapLocksMigration(t *testing.T) {
	safe := SafeDocument{
		Canvas: Canvas{Width: 1920, Height: 1080},
		SafeAreas: []SafeSpan{{
			TimeRange:  TimeRange{0, 2},
			Rect:       Rect{X: 200, Y: 200, W: 1520, H: 680},
			Confidence: conf(0.99),
		}},
	}
	box := Rect{X: 100, Y: 820, W: 700, H: 90}
	doc := &CueDocument{
		Canvas: Canvas{Width: 1920, Height: 1080},
		Cues:   []*Cue{{ID: "gap1", TimeRange: TimeRange{1, 5}, Text: "Across the gap", Box: &box}},
	}
	eng := buildEngine(t, doc, safe)
	rep, _ := eng.Analyze()
	f := findFinding(rep, "gap1")
	if f.Gate.FullCoverage || f.Gate.MigrationAllowed {
		t.Fatalf("coverage gap must lock migration: %+v", f.Gate)
	}
	if _, ar, _ := eng.ApplyAuto(rep); len(ar.Applied) != 0 {
		t.Fatal("must not auto-move with coverage gap")
	}
}
