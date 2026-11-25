package vttlayout

import "testing"

func analyzeExample(t *testing.T, vtt string) (*Document, *Plan) {
	t.Helper()
	doc, err := Parse(vtt)
	if err != nil {
		t.Fatal(err)
	}
	return doc, Analyze(doc, Portrait1080x1920(), Options{})
}

func TestOverlapExampleIsPortraitOnly(t *testing.T) {
	doc, err := Parse(ExampleOverlapVTT)
	if err != nil {
		t.Fatal(err)
	}

	land := Analyze(doc, Landscape1080p(), Options{})
	for _, v := range land.Before {
		if v.Kind == ViolationOverlap {
			t.Fatalf("landscape should have no authored overlap, got: %s", v.Detail)
		}
	}

	port := Analyze(doc, Portrait1080x1920(), Options{})
	overlaps := 0
	for _, v := range port.Before {
		if v.Kind == ViolationOverlap {
			overlaps++
		}
	}
	if overlaps == 0 {
		t.Fatal("portrait example should reproduce at least one region overlap")
	}
	for _, v := range port.Before {
		if v.Kind == ViolationOutOfBounds {
			t.Fatalf("original cues should stay inside the canvas (没有越界), got: %s", v.Detail)
		}
	}
	if !port.Feasible {
		t.Fatalf("solver should repair the overlap example; after=%d combos=%d",
			len(port.After), len(port.Combos))
	}
	if len(port.After) != 0 {
		t.Fatalf("expected zero remaining violations, got %+v", port.After)
	}
}

func TestPriorityKeepsHighCueNearAnchor(t *testing.T) {
	_, port := analyzeExample(t, ExampleOverlapVTT)

	var high, low *Box
	for _, b := range port.Boxes {
		switch b.Cue.Order {
		case 1: // guest priority 8
			high = b
		case 0: // host priority 5
			low = b
		}
	}
	if high == nil || low == nil {
		t.Fatal("boxes missing")
	}
	if absFloat(high.Rect.Y-high.Authored.Y) > 1 {
		t.Fatalf("higher priority cue should keep its anchor, moved %.0fpx",
			high.Rect.Y-high.Authored.Y)
	}
	if !low.Pushed {
		t.Fatal("lower priority cue should yield and be pushed")
	}
	// Final rects must not intersect while their times overlap.
	if high.Rect.Intersects(low.Rect) {
		t.Fatal("high/low boxes still intersect")
	}
}

func TestInfeasibleComboAndCandidates(t *testing.T) {
	doc, plan := analyzeExample(t, ExampleInfeasibleVTT)
	if plan.Feasible {
		t.Fatal("infeasible example must not be feasible")
	}
	if len(plan.Combos) != 1 {
		t.Fatalf("want exactly 1 combo, got %d", len(plan.Combos))
	}
	cb := plan.Combos[0]
	if !cb.Certified || cb.OverflowPx <= 0 {
		t.Fatalf("combo should be certified overflow, got %+v", cb)
	}
	if len(cb.CueOrders) != 4 {
		t.Fatalf("want 4 cue combo, got %v", cb.CueOrders)
	}

	kinds := map[CandidateKind]bool{}
	resolving := map[CandidateKind]bool{}
	for _, c := range plan.Candidates {
		if c.ComboID != cb.ID {
			continue
		}
		kinds[c.Kind] = true
		if c.Resolves {
			resolving[c.Kind] = true
		}
	}
	for _, k := range []CandidateKind{CandidateSplitCue, CandidateExtendTime, CandidateExtendCanvas} {
		if !kinds[k] {
			t.Fatalf("missing candidate kind %s", k)
		}
		if !resolving[k] {
			t.Fatalf("candidate %s should verify as resolving", k)
		}
	}

	// Applying the split candidate must produce a parseable VTT whose
	// re analysis is clean around the combo window.
	var split *Candidate
	for _, c := range plan.Candidates {
		if c.Kind == CandidateSplitCue && c.Resolves {
			split = c
		}
	}
	doc2, err := ApplyCandidate(doc, split)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(Serialize(doc2)); err != nil {
		t.Fatalf("split VTT not parseable: %v", err)
	}
	plan2 := Analyze(doc2, Portrait1080x1920(), Options{})
	for _, other := range plan2.Combos {
		if other.StartMS < cb.EndMS && other.EndMS > cb.StartMS {
			t.Fatalf("split did not remove combo window: %+v", other)
		}
	}
}

func TestDisjointTimesNeverConflict(t *testing.T) {
	doc := &Document{Regions: map[string]*Region{}, Cues: []*Cue{
		{Order: 0, StartMS: 0, EndMS: 1000, Text: "first"},
		{Order: 1, StartMS: 1000, EndMS: 2000, Text: "second"},
	}}
	plan := Analyze(doc, Portrait1080x1920(), Options{})
	if len(plan.Before) != 0 || !plan.Feasible {
		t.Fatalf("sequential cues must not conflict: before=%+v after=%+v",
			plan.Before, plan.After)
	}
}

func TestSideBySideBoxesShareVerticalSpace(t *testing.T) {
	// Two narrow regions at the same height but disjoint X must not push.
	vtt := `WEBVTT

REGION
id:L
width:30%
regionanchor:0%,100%
viewportanchor:7%,86%

REGION
id:R
width:30%
regionanchor:100%,100%
viewportanchor:93%,86%

00:00:01.000 --> 00:00:05.000 region:L
左边

00:00:01.000 --> 00:00:05.000 region:R
右边
`
	_, plan := analyzeExample(t, vtt)
	if len(plan.Before) != 0 {
		t.Fatalf("side by side regions should not overlap: %+v", plan.Before)
	}
	for _, b := range plan.Boxes {
		if b.Pushed {
			t.Fatalf("side box should not be pushed: %+v", b.Rect)
		}
	}
}
