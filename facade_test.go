package safereflow

import "testing"

func TestSafeTrackAtAndAutoReflow(t *testing.T) {
	track, err := NewSafeTrack(SafeDocument{
		Canvas: Canvas{Width: 1920, Height: 1080},
		SafeAreas: []SafeSpan{
			{TimeRange: TimeRange{0, 4}, Rect: Rect{X: 120, Y: 120, W: 1680, H: 860}, Confidence: conf(0.95)},
			{TimeRange: TimeRange{4, 8}, Rect: Rect{X: 420, Y: 200, W: 1080, H: 700}, Confidence: conf(0.5)},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if sp, ok := track.At(2); !ok || sp.ID != "sa-1" {
		t.Fatalf("At(2) = %+v %v", sp, ok)
	}
	if _, ok := track.At(9); ok {
		t.Fatal("At(9) must be uncovered")
	}

	box := Rect{X: 100, Y: 820, W: 700, H: 90}
	doc := &CueDocument{
		Canvas: Canvas{Width: 1920, Height: 1080},
		Cues:   []*Cue{{ID: "a", TimeRange: TimeRange{1, 3}, Text: "Safe-ish text", Box: &box}},
	}
	rep, out, ar, err := AutoReflow(doc, track)
	if err != nil {
		t.Fatal(err)
	}
	if rep == nil || out == nil || len(ar.Applied) != 1 {
		t.Fatalf("auto reflow result wrong: applied=%+v err=%v", ar, err)
	}
	if out.Cues[0].Box.X != 120 {
		t.Fatalf("expected auto migration to x=120, got %v", out.Cues[0].Box.X)
	}
	if doc.Cues[0].Box.X != 100 {
		t.Fatal("AutoReflow mutated the input document")
	}
}
