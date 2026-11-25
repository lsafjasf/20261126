package timeline

import (
	"testing"

	"proxyrelink/internal/match"
	"proxyrelink/internal/media"
)

func testIndex() media.Index {
	return media.Index{Media: []media.MediaInfo{
		{ID: "p1", Kind: "proxy", DurationMs: 8000, FrameCount: 200, FPSNum: 25, FPSDen: 1},
		{ID: "o1", Kind: "original", DurationMs: 8000, FrameCount: 200, FPSNum: 25, FPSDen: 1},
		{ID: "p2", Kind: "proxy", DurationMs: 4000, FrameCount: 100, FPSNum: 25, FPSDen: 1},
		{ID: "o2", Kind: "original", DurationMs: 4040, FrameCount: 101, FPSNum: 25, FPSDen: 1},
	}}
}

func testTimeline() Timeline {
	return Timeline{
		Name: "seq", FPSNum: 25, FPSDen: 1,
		Cuts: []Cut{
			{Seq: 1, ClipID: "p1", InFrame: 10, OutFrame: 190},
			{Seq: 2, ClipID: "p2", InFrame: 0, OutFrame: 100},
			{Seq: 3, ClipID: "gfx", InFrame: 0, OutFrame: 50},
		},
	}
}

func TestRelinkPreservesCutPoints(t *testing.T) {
	ix := testIndex()
	tl := testTimeline()
	dec := &match.Decisions{}
	if err := dec.Lock("p1", "o1", 0.95); err != nil {
		t.Fatal(err)
	}
	rep := Relink(tl, ix, dec, 0)
	if rep.Relinked != 1 || rep.Unchanged != 2 || rep.Errors != 0 {
		t.Fatalf("unexpected counts: %+v", rep)
	}
	cut1 := rep.Cuts[0]
	if cut1.Action != "relinked" || cut1.NewClip != "o1" {
		t.Fatalf("cut 1 should relink to o1: %+v", cut1)
	}
	if cut1.InFrame != 10 || cut1.OutFrame != 190 {
		t.Fatalf("cut points must not move: got [%d,%d)", cut1.InFrame, cut1.OutFrame)
	}
	// p2->o2 is not locked, so cut 2 stays on the proxy.
	if rep.Cuts[1].Action != "kept" {
		t.Fatalf("unlocked proxy must be kept: %+v", rep.Cuts[1])
	}
	// Non-media clip stays untouched.
	if rep.Cuts[2].Action != "kept" {
		t.Fatalf("non-proxy clip must be kept: %+v", rep.Cuts[2])
	}
}

func TestRelinkRejectsFrameCountDrift(t *testing.T) {
	ix := testIndex()
	tl := testTimeline()
	dec := &match.Decisions{}
	if err := dec.Lock("p2", "o2", 0.8); err != nil {
		t.Fatal(err)
	}
	// Strict tolerance: 100 vs 101 frames must fail.
	rep := Relink(tl, ix, dec, 0)
	if rep.Cuts[1].Action != "error" {
		t.Fatalf("frame drift must block relink: %+v", rep.Cuts[1])
	}
	if rep.Cuts[1].InFrame != 0 || rep.Cuts[1].OutFrame != 100 {
		t.Fatal("failed cut must keep original points")
	}
	// With tolerance 1 the swap is allowed.
	rep = Relink(tl, ix, dec, 1)
	if rep.Cuts[1].Action != "relinked" || rep.Cuts[1].NewClip != "o2" {
		t.Fatalf("tolerance 1 should allow relink: %+v", rep.Cuts[1])
	}
}

func TestRelinkRejectsOutOfRangeCut(t *testing.T) {
	ix := testIndex()
	tl := Timeline{
		Name: "seq", FPSNum: 25, FPSDen: 1,
		Cuts: []Cut{{Seq: 1, ClipID: "p1", InFrame: 0, OutFrame: 250}},
	}
	dec := &match.Decisions{}
	if err := dec.Lock("p1", "o1", 0.9); err != nil {
		t.Fatal(err)
	}
	rep := Relink(tl, ix, dec, 0)
	if rep.Cuts[0].Action != "error" {
		t.Fatalf("out-point beyond original must error: %+v", rep.Cuts[0])
	}
}

func TestRelinkRejectsFPSMismatch(t *testing.T) {
	ix := testIndex()
	ix.Media[1].FPSNum = 24 // original at 24fps vs 25fps timeline
	tl := testTimeline()
	dec := &match.Decisions{}
	if err := dec.Lock("p1", "o1", 0.9); err != nil {
		t.Fatal(err)
	}
	rep := Relink(tl, ix, dec, 0)
	if rep.Cuts[0].Action != "error" {
		t.Fatalf("fps mismatch must error: %+v", rep.Cuts[0])
	}
}
