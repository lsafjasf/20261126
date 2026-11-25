package safereflow

import "testing"

func conf(v float64) *float64 { return &v }

func TestNewSafeTrackNormalizesAndValidates(t *testing.T) {
	doc := SafeDocument{
		Canvas: Canvas{Width: 1000, Height: 500},
		Unit:   "normalized",
		SafeAreas: []SafeSpan{{
			TimeRange:  TimeRange{Start: 0, End: 2},
			Rect:       Rect{X: 0.1, Y: 0.2, W: 0.8, H: 0.6},
			Confidence: conf(0.9),
		}},
	}
	track, err := NewSafeTrack(doc)
	if err != nil {
		t.Fatal(err)
	}
	sp := track.Spans()[0]
	want := Rect{X: 100, Y: 100, W: 800, H: 300}
	if !rectNear(sp.Rect, want) {
		t.Fatalf("normalized rect = %+v, want %+v", sp.Rect, want)
	}
	if sp.ID == "" {
		t.Fatal("expected default span id")
	}

	bad := doc
	bad.Canvas = Canvas{}
	if _, err := NewSafeTrack(bad); err == nil {
		t.Fatal("expected canvas validation error")
	}

	badConf := doc
	badConf.Unit = "pixel"
	badConf.SafeAreas = []SafeSpan{{TimeRange: TimeRange{0, 1}, Rect: Rect{W: 10, H: 10}, Confidence: conf(2)}}
	if _, err := NewSafeTrack(badConf); err == nil {
		t.Fatal("expected confidence range error")
	}
}

func TestSlicesOverPartitionsAndReportsGaps(t *testing.T) {
	doc := SafeDocument{
		Canvas: Canvas{Width: 1920, Height: 1080},
		SafeAreas: []SafeSpan{
			{TimeRange: TimeRange{Start: 0, End: 4}, Rect: Rect{X: 100, Y: 100, W: 1720, H: 880}, Confidence: conf(0.95)},
			{TimeRange: TimeRange{Start: 4, End: 8}, Rect: Rect{X: 400, Y: 200, W: 1120, H: 700}, Confidence: conf(0.9)},
		},
	}
	track, err := NewSafeTrack(doc)
	if err != nil {
		t.Fatal(err)
	}

	// Cue crosses the 4s boundary and ends after coverage at 8s.
	slices := track.SlicesOver(TimeRange{Start: 2, End: 9})
	if len(slices) != 3 {
		t.Fatalf("got %d slices: %+v", len(slices), slices)
	}
	if !near(slices[0].Start, 2) || !near(slices[0].End, 4) || !slices[0].Trusted(0.8) {
		t.Fatalf("slice0 unexpected: %+v", slices[0])
	}
	if !near(slices[1].Start, 4) || !near(slices[1].End, 8) || slices[1].Rect.X != 400 {
		t.Fatalf("slice1 unexpected: %+v", slices[1])
	}
	if slices[2].Covered {
		t.Fatalf("slice2 must be a coverage gap: %+v", slices[2])
	}

	common, minConf, known, full, has := track.CommonRectOver(TimeRange{Start: 1, End: 7})
	if !full || !known || !has {
		t.Fatalf("expected full known coverage, got full=%v known=%v has=%v", full, known, has)
	}
	if !near(minConf, 0.9) {
		t.Fatalf("minConf = %v", minConf)
	}
	// Intersection of the two rects: X=400, Y=200, right=1820 vs 1520 -> W=1120,
	// bottom=980 vs 900 -> H=700.
	if common.X != 400 || common.Y != 200 || common.W != 1120 || common.H != 700 {
		t.Fatalf("common rect = %+v", common)
	}

	_, _, _, fullGap, _ := track.CommonRectOver(TimeRange{Start: 2, End: 9})
	if fullGap {
		t.Fatal("expected incomplete coverage")
	}
}

func TestUnknownConfidenceIsNotTrusted(t *testing.T) {
	doc := SafeDocument{
		Canvas: Canvas{Width: 100, Height: 100},
		SafeAreas: []SafeSpan{{
			TimeRange: TimeRange{0, 2},
			Rect:      Rect{X: 0, Y: 0, W: 100, H: 100},
		}},
	}
	track, err := NewSafeTrack(doc)
	if err != nil {
		t.Fatal(err)
	}
	slices := track.SlicesOver(TimeRange{0, 2})
	if !slices[0].Covered || slices[0].ConfidenceKnown {
		t.Fatalf("expected covered but unknown confidence: %+v", slices[0])
	}
	if slices[0].Trusted(0.8) {
		t.Fatal("unknown confidence must not be trusted")
	}
}
