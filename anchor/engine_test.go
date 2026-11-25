package anchor

import "testing"

const testTick Ticks = 1_000_000

func testProject() *Project {
	return &Project{
		Version:        1,
		TicksPerSecond: 1_000_000,
		Media:          []Media{{ID: "m1", Duration: 100 * testTick}},
		Events: []Event{
			{ID: "E1", Kind: KindSubtitle,
				Anchor: &Anchor{MediaID: "m1", Start: 5 * testTick, End: 25 * testTick}},
			{ID: "E2", Kind: KindChapter,
				Anchor: &Anchor{MediaID: "m1", Start: 40 * testTick, End: 45 * testTick}},
			{ID: "LEG", Kind: KindSubtitle, Text: "legacy"},
		},
		Edl: []Segment{
			{ID: "s1", MediaID: "m1", SrcStart: 0, SrcEnd: 10 * testTick, DstStart: 0},
			{ID: "s2", MediaID: "m1", SrcStart: 10 * testTick, SrcEnd: 30 * testTick, DstStart: 100 * testTick},
			// Reused range 5s..10s appears again.
			{ID: "s3", MediaID: "m1", SrcStart: 5 * testTick, SrcEnd: 10 * testTick, DstStart: 200 * testTick},
		},
	}
}

func findPiece(t *testing.T, me MappedEvent, key string) Piece {
	t.Helper()
	for _, piece := range me.Pieces {
		if piece.Key == key {
			return piece
		}
	}
	t.Fatalf("piece %q not found", key)
	return Piece{}
}

func findMapped(r *Result, eventID string) MappedEvent {
	for _, me := range r.Mapped {
		if me.EventID == eventID {
			return me
		}
	}
	return MappedEvent{}
}

func TestMapSplitsAndFlagsAmbiguity(t *testing.T) {
	p := testProject()
	r, err := p.Map()
	if err != nil {
		t.Fatal(err)
	}
	me := findMapped(r, "E1")
	if len(me.Pieces) != 2 {
		t.Fatalf("expected 2 split pieces, got %d", len(me.Pieces))
	}

	ambiguousKey := PieceKey("E1", Anchor{MediaID: "m1", Start: 5 * testTick, End: 10 * testTick})
	amb := findPiece(t, me, ambiguousKey)
	if amb.Status != StatusAmbiguous || len(amb.Candidates) != 2 {
		t.Fatalf("expected ambiguous piece with 2 candidates, got %+v", amb)
	}
	if amb.Candidates[0].SegmentID != "s1" || amb.Candidates[1].SegmentID != "s3" {
		t.Fatalf("candidates not deterministically ordered: %+v", amb.Candidates)
	}

	mappedKey := PieceKey("E1", Anchor{MediaID: "m1", Start: 10 * testTick, End: 25 * testTick})
	mapped := findPiece(t, me, mappedKey)
	if mapped.Status != StatusMapped || mapped.SegmentID != "s2" {
		t.Fatalf("expected mapped through s2, got %+v", mapped)
	}
	// Source 10s..25s lands at destination 100s..115s.
	if mapped.DstStart != 100*testTick || mapped.DstEnd != 115*testTick {
		t.Fatalf("unexpected destination: %d..%d", mapped.DstStart, mapped.DstEnd)
	}
}

func TestRemovedFragment(t *testing.T) {
	p := testProject()
	r, err := p.Map()
	if err != nil {
		t.Fatal(err)
	}
	me := findMapped(r, "E2")
	if len(me.Pieces) != 1 || me.Pieces[0].Status != StatusRemoved {
		t.Fatalf("expected one removed piece, got %+v", me.Pieces)
	}
}

func TestManualSegmentResolution(t *testing.T) {
	p := testProject()
	key := PieceKey("E1", Anchor{MediaID: "m1", Start: 5 * testTick, End: 10 * testTick})
	if err := p.SetSegmentDecision(key, "s3", "pick flashback"); err != nil {
		t.Fatal(err)
	}
	r, err := p.Map()
	if err != nil {
		t.Fatal(err)
	}
	piece := findPiece(t, findMapped(r, "E1"), key)
	if piece.Status != StatusMapped || piece.SegmentID != "s3" {
		t.Fatalf("decision not applied: %+v", piece)
	}
	if piece.DstStart != 200*testTick {
		t.Fatalf("expected flashback destination, got %d", piece.DstStart)
	}

	// Choosing a segment that is not a candidate must be rejected.
	if err := p.SetSegmentDecision(key, "s2", "bad"); err == nil {
		t.Fatal("expected rejection of non-candidate segment")
	}
}

func TestDropAndCustomDecisions(t *testing.T) {
	p := testProject()
	key := PieceKey("E2", Anchor{MediaID: "m1", Start: 40 * testTick, End: 45 * testTick})

	if err := p.SetDropDecision(key, "cut"); err != nil {
		t.Fatal(err)
	}
	r, _ := p.Map()
	piece := findPiece(t, findMapped(r, "E2"), key)
	if piece.Status != StatusMapped || !piece.Dropped {
		t.Fatalf("expected dropped mapped piece, got %+v", piece)
	}

	if err := p.RemoveDecision(key); err != nil {
		t.Fatal(err)
	}
	if err := p.SetCustomDecision(key, 50*testTick, 55*testTick, "relocated"); err != nil {
		t.Fatal(err)
	}
	r, _ = p.Map()
	piece = findPiece(t, findMapped(r, "E2"), key)
	if piece.DstStart != 50*testTick || piece.DstEnd != 55*testTick {
		t.Fatalf("custom decision not applied: %+v", piece)
	}
}

func TestLegacyAnchorDecision(t *testing.T) {
	p := testProject()
	r, err := p.Map()
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Unresolved) != 3 {
		t.Fatalf("expected ambiguity, removed and legacy unresolved items, got %d", len(r.Unresolved))
	}

	if err := p.SetAnchorDecision("LEG", "m1", 6*testTick, 8*testTick, "recovered"); err != nil {
		t.Fatal(err)
	}
	r, err = p.Map()
	if err != nil {
		t.Fatal(err)
	}
	me := findMapped(r, "LEG")
	if len(me.Pieces) != 1 || me.Pieces[0].Status != StatusAmbiguous {
		t.Fatalf("anchored legacy event should now map into reused range: %+v", me.Pieces)
	}

	// Invalid anchor interval must be rejected.
	if err := p.SetAnchorDecision("LEG", "m1", 9*testTick, 9*testTick, "bad"); err == nil {
		t.Fatal("expected rejection of empty anchor interval")
	}
	if err := p.SetAnchorDecision("LEG", "unknown", 0, testTick, "bad"); err == nil {
		t.Fatal("expected rejection of unknown media")
	}
}

func TestStaleDecisionAfterEdit(t *testing.T) {
	p := testProject()
	key := PieceKey("E1", Anchor{MediaID: "m1", Start: 5 * testTick, End: 10 * testTick})
	p.Decisions = map[string]Decision{
		key: {Action: ActionSegment, SegmentID: "s3", Reason: "flashback"},
	}
	// Re-cut removes segment s3: the same fragment now maps uniquely to s1.
	p.Edl = p.Edl[:2]
	r, err := p.Map()
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Stale) != 1 || r.Stale[0].Key != key {
		t.Fatalf("expected one stale decision, got %+v", r.Stale)
	}
	piece := findPiece(t, findMapped(r, "E1"), key)
	if piece.Status != StatusMapped || piece.SegmentID != "s1" {
		t.Fatalf("fragment should auto-map to the remaining segment, got %+v", piece)
	}
}

func TestStaleAnchorDecisionForDeletedEvent(t *testing.T) {
	p := testProject()
	p.AnchorDecisions = map[string]AnchorDecision{
		"LEG": {MediaID: "m1", Start: 6 * testTick, End: 8 * testTick, Reason: "recovered"},
	}
	p.Events = p.Events[:2]
	r, err := p.Map()
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Stale) != 1 || r.Stale[0].Key != "anchor:LEG" {
		t.Fatalf("expected stale anchor decision, got %+v", r.Stale)
	}
}
