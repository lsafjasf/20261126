package subselect

import (
	"testing"
	"time"
)

func TestRebase_UsesTrackTimeBase(t *testing.T) {
	track, ok := fixtureCatalog().Get("en-main")
	if !ok {
		t.Fatal("missing en-main")
	}
	sched, err := Rebase(track, track.Cues)
	if err != nil {
		t.Fatal(err)
	}

	// en-2 is 405000..675000 at 90 kHz => 4.5s..7.5s.
	cue := sched.Cues[1]
	if cue.ID != "en-2" || cue.Start != 4500*time.Millisecond || cue.End != 7500*time.Millisecond {
		t.Fatalf("unexpected rebased cue: %+v", cue)
	}
	if cue.Language != "en" || cue.TrackID != "en-main" {
		t.Fatalf("rebased cue lost track identity: %+v", cue)
	}
}

func TestSchedule_ActiveHalfOpenBoundaries(t *testing.T) {
	track, _ := fixtureCatalog().Get("zh-main")
	sched, err := Rebase(track, track.Cues)
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := sched.Active(999 * time.Millisecond); ok {
		t.Fatal("cue must not start before Start")
	}
	got, ok := sched.Active(1 * time.Second)
	if !ok || got.ID != "zh-main-1" {
		t.Fatalf("start boundary: got %+v ok=%v", got, ok)
	}
	if _, ok := sched.Active(3 * time.Second); ok {
		t.Fatal("end boundary is exclusive")
	}
	got, ok = sched.Active(5 * time.Second)
	if !ok || got.ID != "zh-main-2" {
		t.Fatalf("middle cue: got %+v ok=%v", got, ok)
	}
	if _, ok := sched.Active(10 * time.Second); ok {
		t.Fatal("position after last cue must be empty")
	}
}

func TestRebase_RejectsZeroTimeBase(t *testing.T) {
	_, err := Rebase(Track{ID: "x"}, []Cue{{Start: 1, End: 2}})
	if err != ErrInvalidTimeBase {
		t.Fatalf("want ErrInvalidTimeBase, got %v", err)
	}
}
