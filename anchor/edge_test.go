package anchor

import "testing"

func TestCrossMediaSegmentsDoNotMix(t *testing.T) {
	p := &Project{
		Version:        1,
		TicksPerSecond: 1_000_000,
		Media: []Media{
			{ID: "a", Duration: 50 * testTick},
			{ID: "b", Duration: 50 * testTick},
		},
		Events: []Event{
			{ID: "X", Kind: KindSubtitle,
				Anchor: &Anchor{MediaID: "a", Start: 0, End: 5 * testTick}},
		},
		Edl: []Segment{
			// Same numeric source range, different media.
			{ID: "sa", MediaID: "a", SrcStart: 0, SrcEnd: 10 * testTick, DstStart: 0},
			{ID: "sb", MediaID: "b", SrcStart: 0, SrcEnd: 10 * testTick, DstStart: 100 * testTick},
		},
	}
	r, err := p.Map()
	if err != nil {
		t.Fatal(err)
	}
	me := findMapped(r, "X")
	if len(me.Pieces) != 1 {
		t.Fatalf("expected one piece, got %d", len(me.Pieces))
	}
	if me.Pieces[0].SegmentID != "sa" {
		t.Fatalf("cross-media segment leaked into mapping: %+v", me.Pieces[0])
	}
}

func TestPieceKeyRoundTrip(t *testing.T) {
	anchor := Anchor{MediaID: "m1", Start: 100, End: 200}
	key := PieceKey("EV1", anchor)
	eventID, parsed, ok := parsePieceKey(key)
	if !ok {
		t.Fatal("failed to parse piece key")
	}
	if eventID != "EV1" || parsed != anchor {
		t.Fatalf("round trip mismatch: %s %+v", eventID, parsed)
	}

	for _, bad := range []string{"", "nodhash", "e#", "e#m:", "e#m:1-", "e#m:x-y"} {
		if _, _, ok := parsePieceKey(bad); ok {
			t.Fatalf("bad key %q parsed successfully", bad)
		}
	}
}

func TestPieceKeysAreStableAcrossRecuts(t *testing.T) {
	p := DemoProject()
	first, err := p.Map()
	if err != nil {
		t.Fatal(err)
	}

	// Move seg-2 somewhere else on the timeline without touching source ranges.
	for i := range p.Edl {
		if p.Edl[i].ID == "seg-2" {
			p.Edl[i].DstStart += 999 * testTick
		}
	}
	second, err := p.Map()
	if err != nil {
		t.Fatal(err)
	}

	keys := func(r *Result) map[string]bool {
		out := map[string]bool{}
		for _, me := range r.Mapped {
			for _, piece := range me.Pieces {
				out[piece.Key] = true
			}
		}
		return out
	}
	if len(keys(first)) != len(keys(second)) {
		t.Fatal("piece key set changed when only destination positions moved")
	}
	for k := range keys(first) {
		if !keys(second)[k] {
			t.Fatalf("piece key %s disappeared after a destination-only edit", k)
		}
	}
}
