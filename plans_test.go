package safereflow

import (
	"strings"
	"testing"
)

func TestRewrapPlanRecommendedAndIdempotent(t *testing.T) {
	st := DefaultStyle()
	st.MarginY = 100 // bottom anchor at 980, matching safe rect bottom
	doc := &CueDocument{
		Canvas: Canvas{Width: 1920, Height: 1080},
		Style:  st,
		Cues: []*Cue{{
			ID:        "w1",
			TimeRange: TimeRange{Start: 1, End: 3},
			Text:      strings.Repeat("A", 80),
		}},
	}
	safeRect := Rect{X: 160, Y: 120, W: 1600, H: 860}
	eng := buildEngine(t, doc, singleSafe(0.95, safeRect))
	rep, err := eng.Analyze()
	if err != nil {
		t.Fatal(err)
	}
	f := findFinding(rep, "w1")
	if f.Status != StatusViolation {
		t.Fatalf("status = %s box=%+v", f.Status, f.Box)
	}
	rp := findPlan(f, ActionRewrap)
	if rp == nil || !rp.Feasible {
		t.Fatalf("rewrap plan missing: %+v", rp)
	}
	if rp.Move != nil {
		t.Fatalf("pure rewrap should not move anchor, got %+v", rp.Move)
	}
	if !rp.AutoEligible || f.RecommendedPlanID != rp.ID {
		t.Fatalf("rewrap should be recommended auto: rec=%s auto=%v", f.RecommendedPlanID, rp.AutoEligible)
	}
	if !strings.Contains(rp.Text, "\n") {
		t.Fatalf("expected wrapped multi-line text, got %q", rp.Text)
	}

	applied, _, err := eng.ApplyAuto(rep)
	if err != nil {
		t.Fatal(err)
	}
	eng2 := buildEngine(t, applied, singleSafe(0.95, safeRect))
	rep2, _ := eng2.Analyze()
	f2 := findFinding(rep2, "w1")
	if f2.Status != StatusSafe {
		t.Fatalf("post-rewrap status=%s box=%+v overflow segments=%+v", f2.Status, f2.Box, f2.Segments)
	}
}

func TestShortenPlanRequiresExplicitSelection(t *testing.T) {
	st := DefaultStyle()
	st.FontSize = 40
	st.MaxLines = 2
	st.MarginY = 380 // bottom anchor 700 == safe bottom
	doc := &CueDocument{
		Canvas: Canvas{Width: 1920, Height: 1080},
		Style:  st,
		Cues: []*Cue{{
			ID:        "s1",
			TimeRange: TimeRange{Start: 1, End: 3},
			Text:      strings.Repeat("字", 120),
		}},
	}
	safeRect := Rect{X: 660, Y: 300, W: 600, H: 400} // center 960, bottom 700
	eng := buildEngine(t, doc, singleSafe(0.95, safeRect))
	rep, _ := eng.Analyze()
	f := findFinding(rep, "s1")

	rp := findPlan(f, ActionRewrap)
	if rp != nil && rp.Feasible {
		t.Fatal("rewrap must be infeasible under MaxLines=2 for 120 wide runes")
	}
	sp := findPlan(f, ActionShorten)
	if sp == nil || !sp.Feasible {
		t.Fatalf("shorten plan missing: %+v", sp)
	}
	if !sp.ContentLoss || sp.AutoEligible {
		t.Fatal("shorten must be content-lossy and not auto eligible")
	}
	if !strings.HasSuffix(sp.Text, ellipsis) {
		t.Fatalf("short text should end with ellipsis: %q", sp.Text)
	}

	// Automatic path must not shorten.
	autoDoc, ar, _ := eng.ApplyAuto(rep)
	if len(ar.Applied) != 0 || autoDoc.Cues[0].Text != strings.Repeat("字", 120) {
		t.Fatal("auto path must not apply content-loss shortening")
	}

	// Explicit operator selection applies it.
	applied, appliedRep, err := eng.Apply(rep, ApplyOptions{PlanIDs: []string{sp.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if len(appliedRep.Applied) != 1 {
		t.Fatalf("applied report = %+v", appliedRep)
	}
	eng2 := buildEngine(t, applied, singleSafe(0.95, safeRect))
	rep2, _ := eng2.Analyze()
	if got := findFinding(rep2, "s1").Status; got != StatusSafe {
		t.Fatalf("post-shorten status=%s box=%+v", got, findFinding(rep2, "s1").Box)
	}
}

func TestTimeSplitForDriftingSafeArea(t *testing.T) {
	safe := SafeDocument{
		Canvas: Canvas{Width: 1920, Height: 1080},
		SafeAreas: []SafeSpan{
			{TimeRange: TimeRange{0, 2}, Rect: Rect{X: 100, Y: 700, W: 700, H: 300}, Confidence: conf(0.95)},
			{TimeRange: TimeRange{2, 4}, Rect: Rect{X: 1120, Y: 200, W: 700, H: 300}, Confidence: conf(0.92)},
		},
	}
	box := Rect{X: 150, Y: 800, W: 300, H: 90} // inside first, far from second
	doc := &CueDocument{
		Canvas: Canvas{Width: 1920, Height: 1080},
		Cues:   []*Cue{{ID: "t1", TimeRange: TimeRange{Start: 0.5, End: 3.5}, Text: "Drifting safe area", Box: &box}},
	}
	eng := buildEngine(t, doc, safe)
	rep, _ := eng.Analyze()
	f := findFinding(rep, "t1")

	mp := findPlan(f, ActionMigrate)
	if mp != nil && mp.Feasible {
		t.Fatal("single migrate should be infeasible when rects do not overlap enough")
	}
	tsp := findPlan(f, ActionTimeSplit)
	if tsp == nil || !tsp.Feasible || !tsp.AutoEligible {
		t.Fatalf("time_split plan missing: %+v", tsp)
	}
	if len(tsp.Segments) != 2 {
		t.Fatalf("segments = %+v", tsp.Segments)
	}
	if !near(tsp.Segments[1].DX, 970) || !near(tsp.Segments[1].DY, -390) {
		t.Fatalf("second segment move = %+v", tsp.Segments[1])
	}
	if f.RecommendedPlanID != tsp.ID {
		t.Fatalf("recommend = %s", f.RecommendedPlanID)
	}

	applied, _, err := eng.ApplyAuto(rep)
	if err != nil {
		t.Fatal(err)
	}
	if len(applied.Cues) != 2 {
		t.Fatalf("expected split into 2 cues, got %d", len(applied.Cues))
	}
	if applied.Cues[1].ID != "t1#2" || applied.Cues[1].Box == nil || applied.Cues[1].Box.X != 1120 {
		t.Fatalf("second sub-cue wrong: %+v", applied.Cues[1])
	}

	eng2 := buildEngine(t, applied, safe)
	rep2, _ := eng2.Analyze()
	for _, id := range []string{"t1#1", "t1#2"} {
		if got := findFinding(rep2, id); got == nil || got.Status != StatusSafe {
			t.Fatalf("%s not safe after split: %+v", id, got)
		}
	}
}

func TestShortenTextWordBoundary(t *testing.T) {
	got := shortenText("hello wonderful world", 12, len([]rune("hello wonderful world")))
	if !strings.HasSuffix(got, ellipsis) {
		t.Fatalf("expected ellipsis: %q", got)
	}
	if strings.Contains(got, "wonderf") {
		t.Fatalf("should back off to word boundary, got %q", got)
	}
}
