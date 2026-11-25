package subselect

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestPlayer_SwitchCommitsConsistentView(t *testing.T) {
	cat, sel := fixtureSelector("en")
	clock := NewFakeClock(time.Unix(1_700_000_000, 0))
	p := NewPlayer("b1", sel, WithClock(clock))

	result, err := p.Switch(context.Background(), Criteria{Language: "zh"})
	if err != nil {
		t.Fatal(err)
	}
	if result.View.Generation != 1 {
		t.Fatalf("first switch generation = %d, want 1", result.View.Generation)
	}
	if err := checkViewConsistency(cat, result.View); err != nil {
		t.Fatal(err)
	}

	result, err = p.Switch(context.Background(), Criteria{Language: "en", Version: "sdh"})
	if err != nil {
		t.Fatal(err)
	}
	if result.View.Decision.Track.ID != "en-sdh" {
		t.Fatalf("got %q, want en-sdh", result.View.Decision.Track.ID)
	}
	if result.View.Generation != 2 {
		t.Fatalf("second switch generation = %d, want 2", result.View.Generation)
	}
	if err := checkViewConsistency(cat, result.View); err != nil {
		t.Fatal(err)
	}
}

func TestPlayer_StaleAsyncSwitchCannotOverwriteNewerSelection(t *testing.T) {
	slow := Track{
		ID: "slow", Language: "zh", TimeBase: TimeBase{Timescale: 1000},
		Cues: []Cue{{ID: "slow-1", Start: 0, End: 1000, Text: "慢"}},
	}
	fast := Track{
		ID: "fast", Language: "en", TimeBase: TimeBase{Timescale: 1000},
		Cues: []Cue{{ID: "fast-1", Start: 0, End: 1000, Text: "fast"}},
	}
	cat, err := NewCatalog(slow, fast)
	if err != nil {
		t.Fatal(err)
	}
	sel := NewSelector(cat, SelectorConfig{})

	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	loader := LoadFunc(func(_ context.Context, tr Track) ([]Cue, error) {
		if tr.ID == "slow" {
			once.Do(func() { close(entered) })
			<-release
		}
		return tr.Cues, nil
	})

	p := NewPlayer("race", sel, WithLoader(loader))

	type outcome struct {
		result SwitchResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		r, err := p.Switch(context.Background(), Criteria{TrackID: "slow"})
		done <- outcome{r, err}
	}()

	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("slow switch never entered loader")
	}

	// Newer switch completes while the older one is still loading.
	newer, err := p.Switch(context.Background(), Criteria{TrackID: "fast"})
	if err != nil {
		t.Fatalf("newer switch: %v", err)
	}
	if newer.View.Decision.Track.ID != "fast" {
		t.Fatalf("newer switch committed %q", newer.View.Decision.Track.ID)
	}

	close(release)
	select {
	case out := <-done:
		if !errors.Is(out.err, ErrStaleSwitch) {
			t.Fatalf("older switch err = %v, want ErrStaleSwitch", out.err)
		}
		if !out.result.Stale {
			t.Fatal("older result must be marked stale")
		}
		if out.result.View == nil || out.result.View.Decision.Track.ID != "fast" {
			t.Fatalf("stale result must return current fast view, got %+v", out.result.View)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("stale switch did not return")
	}

	snap := p.Snapshot()
	if snap.Decision.Track.ID != "fast" || snap.Schedule.TrackID != "fast" {
		t.Fatalf("stale load overwrote selection: track=%q schedule=%q",
			snap.Decision.Track.ID, snap.Schedule.TrackID)
	}
	if err := checkViewConsistency(cat, snap); err != nil {
		t.Fatal(err)
	}
}

func TestPlayer_LoadFailureFallsBackAlongChain(t *testing.T) {
	cat, sel := fixtureSelector("en")
	loader := LoadFunc(func(_ context.Context, tr Track) ([]Cue, error) {
		if tr.ID == "zh-main" {
			return nil, errors.New("404 manifest missing")
		}
		return tr.Cues, nil
	})
	p := NewPlayer("b2", sel, WithLoader(loader))

	result, err := p.Switch(context.Background(), Criteria{Language: "zh"})
	if err != nil {
		t.Fatalf("fallback should recover: %v", err)
	}
	if result.View.Decision.Track.ID != "zh-director" {
		t.Fatalf("recovered track = %q, want zh-director", result.View.Decision.Track.ID)
	}
	if result.Decision.Reason != ReasonLoadFallback {
		t.Fatalf("reason = %q, want load_fallback", result.Decision.Reason)
	}
	if len(result.LoadErrors) != 1 || result.LoadErrors[0].TrackID != "zh-main" {
		t.Fatalf("load errors = %+v", result.LoadErrors)
	}
	if err := checkViewConsistency(cat, result.View); err != nil {
		t.Fatal(err)
	}
}

func TestPlayer_AllCandidatesFail_CommitsExplicitOff(t *testing.T) {
	cat, sel := fixtureSelector()
	boom := errors.New("origin down")
	loader := LoadFunc(func(context.Context, Track) ([]Cue, error) {
		return nil, boom
	})
	p := NewPlayer("b3", sel, WithLoader(loader))

	result, err := p.Switch(context.Background(), Criteria{Language: "en"})
	if !errors.Is(err, ErrNoAvailableTrack) || !errors.Is(err, boom) {
		t.Fatalf("err = %v, want joined ErrNoAvailableTrack + boom", err)
	}
	if !result.View.Off || result.View.Schedule != nil {
		t.Fatalf("exhausted chain must commit off view: %+v", result.View)
	}
	snap := p.Snapshot()
	if err := checkViewConsistency(cat, snap); err != nil {
		t.Fatal(err)
	}
}

func TestPlayer_ConcurrentSwitchesAndSnapshotsAreConsistent(t *testing.T) {
	cat, sel := fixtureSelector("en")
	p := NewPlayer("hammer", sel)

	criteria := []Criteria{
		{Language: "zh"},
		{Language: "en"},
		{Language: "zh", Version: "director-cut"},
		{Language: "en", HearingImpaired: Yes},
		{TrackID: "en-forced"},
		{Language: "ja"},
	}

	var writers sync.WaitGroup
	for g := 0; g < 8; g++ {
		writers.Add(1)
		go func(seed int) {
			defer writers.Done()
			for i := 0; i < 100; i++ {
				c := criteria[(i+seed)%len(criteria)]
				result, err := p.Switch(context.Background(), c)
				if err != nil &&
					!errors.Is(err, ErrStaleSwitch) &&
					!errors.Is(err, ErrNoAvailableTrack) {
					t.Errorf("unexpected switch error: %v", err)
					return
				}
				if result.View != nil {
					if cerr := checkViewConsistency(cat, result.View); cerr != nil {
						t.Errorf("inconsistent switch view: %v", cerr)
						return
					}
				}
			}
		}(g)
	}

	stop := make(chan struct{})
	var readers sync.WaitGroup
	for g := 0; g < 4; g++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				if err := checkViewConsistency(cat, p.Snapshot()); err != nil {
					t.Errorf("inconsistent snapshot: %v", err)
					return
				}
			}
		}()
	}

	writers.Wait()
	close(stop)
	readers.Wait()

	if err := checkViewConsistency(cat, p.Snapshot()); err != nil {
		t.Fatal(err)
	}
}
