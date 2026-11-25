package vttlayout

import "testing"

func TestFixedVTTRoundTrip(t *testing.T) {
	doc, plan := analyzeExample(t, ExampleOverlapVTT)
	vtt := WriteFixedVTT(plan)
	doc2, err := Parse(vtt)
	if err != nil {
		t.Fatalf("fixed vtt not parseable: %v\n%s", err, vtt)
	}
	if len(doc2.Cues) != len(doc.Cues) {
		t.Fatalf("cue count changed: %d -> %d", len(doc.Cues), len(doc2.Cues))
	}
	if len(doc2.RegionOrder) != len(plan.Boxes) {
		t.Fatalf("want one region per box (%d), got %d", len(plan.Boxes), len(doc2.RegionOrder))
	}
	for _, c := range doc2.Cues {
		if c.Settings.Region == "" || doc2.Regions[c.Settings.Region] == nil {
			t.Fatalf("cue %d has bad region ref %q", c.Order, c.Settings.Region)
		}
	}

	// Re analyzing the serialized fixed VTT must stay feasible.
	plan2 := Analyze(doc2, Portrait1080x1920(), Options{})
	if !plan2.Feasible {
		t.Fatalf("round tripped fixed VTT not feasible: after=%d combos=%d",
			len(plan2.After), len(plan2.Combos))
	}
}

func TestApplyTimeCandidate(t *testing.T) {
	doc, plan := analyzeExample(t, ExampleInfeasibleVTT)
	var timeCand *Candidate
	for _, c := range plan.Candidates {
		if c.Kind == CandidateExtendTime && c.Resolves {
			timeCand = c
		}
	}
	if timeCand == nil {
		t.Fatal("no resolving time candidate")
	}
	doc2, err := ApplyCandidate(doc, timeCand)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range doc2.Cues {
		if c.Order == timeCand.TargetOrder && c.StartMS == timeCand.NewStartMS {
			found = true
		}
	}
	if !found {
		t.Fatal("moved cue not found in applied document")
	}
	if _, err := Parse(Serialize(doc2)); err != nil {
		t.Fatal(err)
	}
}

func TestFindSplitPoint(t *testing.T) {
	idx, desc := FindSplitPoint("第一句话说完了，然后是第二句话。")
	if idx <= 0 || idx >= len([]rune("第一句话说完了，然后是第二句话。")) {
		t.Fatalf("bad split index %d (%s)", idx, desc)
	}
	a, b := splitAt("第一句话说完了，然后是第二句话。", idx)
	if a == "" || b == "" {
		t.Fatal("split produced empty part")
	}
}

func splitAt(s string, idx int) (string, string) {
	r := []rune(s)
	return string(r[:idx]), string(r[idx:])
}
