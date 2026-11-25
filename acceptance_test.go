package subselect

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestAcceptance_TwoBrowsers_ConcurrentLanguageSwitch reproduces the reported
// incident at the domain level: two browsers share one catalog/selector but
// own independent sessions, and switch languages simultaneously. Every
// observed view must (a) belong to that browser, (b) stay in the language
// that browser asked for, and (c) pair text with timing rebased from that
// same track/time base.
func TestAcceptance_TwoBrowsers_ConcurrentLanguageSwitch(t *testing.T) {
	cat, sel := fixtureSelector("en")
	clock := NewFakeClock(time.Unix(1_700_000_000, 0))

	browserA := NewPlayer("browser-A", sel, WithClock(clock))
	browserB := NewPlayer("browser-B", sel, WithClock(clock))

	aRequests := []Criteria{
		{Language: "zh"},
		{Language: "zh-CN", Version: "theatrical"},
		{Language: "zh", Version: "director-cut"},
	}
	bRequests := []Criteria{
		{Language: "en"},
		{Language: "en", Version: "sdh", HearingImpaired: Yes},
		{Language: "en", HearingImpaired: No},
	}

	var wg sync.WaitGroup
	runBrowser := func(p *Player, requests []Criteria, wantBaseLang string) {
		defer wg.Done()
		for round := 0; round < 150; round++ {
			criteria := requests[round%len(requests)]
			result, err := p.Switch(context.Background(), criteria)
			if err != nil && !errors.Is(err, ErrStaleSwitch) {
				t.Errorf("%s switch: %v", p.ID(), err)
				return
			}
			view := result.View
			if view == nil {
				view = p.Snapshot()
			}
			if view.SessionID != p.ID() {
				t.Errorf("session leakage: view id %q, want %q", view.SessionID, p.ID())
				return
			}
			if view.Off || view.Decision.Track == nil {
				t.Errorf("%s unexpectedly off", p.ID())
				return
			}
			if got := baseLang(view.Decision.Track.Language); got != wantBaseLang {
				t.Errorf("%s observed language %q, want %q (shared selector state leaked)",
					p.ID(), got, wantBaseLang)
				return
			}
			if err := checkViewConsistency(cat, view); err != nil {
				t.Errorf("%s inconsistent view: %v", p.ID(), err)
				return
			}
			if err := checkViewConsistency(cat, p.Snapshot()); err != nil {
				t.Errorf("%s inconsistent snapshot: %v", p.ID(), err)
				return
			}
		}
	}

	wg.Add(2)
	go runBrowser(browserA, aRequests, "zh")
	go runBrowser(browserB, bRequests, "en")

	// Concurrent readers make the "new text + old timing" window observable
	// if views are not published atomically.
	stopReaders := make(chan struct{})
	var readers sync.WaitGroup
	for _, p := range []*Player{browserA, browserB} {
		readers.Add(1)
		go func(p *Player) {
			defer readers.Done()
			for {
				select {
				case <-stopReaders:
					return
				default:
				}
				v := p.Snapshot()
				if v.Decision.Track == nil {
					continue
				}
				if err := checkViewConsistency(cat, v); err != nil {
					t.Errorf("reader %s: %v", p.ID(), err)
					return
				}
			}
		}(p)
	}

	wg.Wait()
	close(stopReaders)
	readers.Wait()
}

// TestAcceptance_SwitchWhilePaused_ShowsNewLanguageWithRebasedTiming is the
// exact user-visible bug: paused at 5s, switch zh -> en. The displayed cue
// must be the English cue active at 5s with the English 90 kHz time base,
// not English text attached to the Chinese cue's 4s..7s window.
func TestAcceptance_SwitchWhilePaused_ShowsNewLanguageWithRebasedTiming(t *testing.T) {
	cat, sel := fixtureSelector()
	clock := NewFakeClock(time.Unix(1_700_000_000, 0))
	p := NewPlayer("paused-browser", sel, WithClock(clock))

	p.Play()
	clock.Advance(5 * time.Second)
	if pos := p.Pause(); pos != 5*time.Second {
		t.Fatalf("pause position = %v, want 5s", pos)
	}

	result, err := p.Switch(context.Background(), Criteria{Language: "en"})
	if err != nil {
		t.Fatal(err)
	}
	view := result.View
	if view.Status != StatusPaused {
		t.Fatalf("status after paused switch = %q, want paused", view.Status)
	}
	if view.Position != 5*time.Second {
		t.Fatalf("switch moved position to %v", view.Position)
	}
	if view.Cue == nil {
		t.Fatal("expected active English cue at 5s")
	}
	if view.Cue.ID != "en-2" || view.Cue.Text != "Let's go" {
		t.Fatalf("active cue = %+v, want en-2/Let's go", view.Cue)
	}
	if view.Cue.Start != 4500*time.Millisecond || view.Cue.End != 7500*time.Millisecond {
		t.Fatalf("English cue timing = %v..%v, want 4.5s..7.5s from 90kHz rebase (stale old time base)",
			view.Cue.Start, view.Cue.End)
	}
	if strings.Contains(view.Cue.Text, "我们走吧") {
		t.Fatal("new language view still shows old language text")
	}
	if err := checkViewConsistency(cat, view); err != nil {
		t.Fatal(err)
	}

	// Staying paused for an hour must not move position or change the cue.
	clock.Advance(time.Hour)
	frozen := p.Snapshot()
	if frozen.Position != 5*time.Second {
		t.Fatalf("paused position drifted to %v", frozen.Position)
	}
	if frozen.Cue == nil || frozen.Cue.ID != "en-2" {
		t.Fatalf("paused cue changed: %+v", frozen.Cue)
	}
	if err := checkViewConsistency(cat, frozen); err != nil {
		t.Fatal(err)
	}
}

// TestAcceptance_PauseAndResume_PreservesSelectionAndProgress walks
// play -> pause -> switch -> long pause -> resume -> pause -> switch -> resume.
func TestAcceptance_PauseAndResume_PreservesSelectionAndProgress(t *testing.T) {
	cat, sel := fixtureSelector()
	clock := NewFakeClock(time.Unix(1_700_000_000, 0))
	p := NewPlayer("pr-browser", sel, WithClock(clock))

	p.Play()
	clock.Advance(2 * time.Second)
	p.Pause()

	// Switch to the offset director-cut version while paused at 2s.
	if _, err := p.Switch(context.Background(), Criteria{
		Language: "zh", Version: "director-cut",
	}); err != nil {
		t.Fatal(err)
	}
	at2s := p.Snapshot()
	if at2s.Position != 2*time.Second {
		t.Fatalf("position = %v, want 2s", at2s.Position)
	}
	if at2s.Decision.Track.ID != "zh-director" {
		t.Fatalf("track = %q", at2s.Decision.Track.ID)
	}
	if at2s.Cue == nil || at2s.Cue.ID != "zh-dir-1" {
		t.Fatalf("director-cut cue at 2s = %+v", at2s.Cue)
	}
	// 1200ms + 500ms offset = 1.7s; 3200ms + 500ms = 3.7s.
	if at2s.Cue.Start != 1700*time.Millisecond || at2s.Cue.End != 3700*time.Millisecond {
		t.Fatalf("offset rebase wrong: %v..%v", at2s.Cue.Start, at2s.Cue.End)
	}

	// Long wall-clock gap while paused must not be credited on resume basis.
	clock.Advance(10 * time.Minute)
	p.Resume()
	clock.Advance(3500 * time.Millisecond)

	at5500 := p.Snapshot()
	if at5500.Status != StatusPlaying {
		t.Fatalf("status after resume = %q", at5500.Status)
	}
	if at5500.Position != 5500*time.Millisecond {
		t.Fatalf("resume progress = %v, want 5.5s (paused interval leaked)", at5500.Position)
	}
	if at5500.Decision.Track.ID != "zh-director" {
		t.Fatal("selection lost across pause/resume")
	}
	if at5500.Cue == nil || at5500.Cue.ID != "zh-dir-2" {
		t.Fatalf("expected zh-dir-2 at 5.5s, got %+v", at5500.Cue)
	}
	if at5500.Cue.Start != 5100*time.Millisecond || at5500.Cue.End != 8100*time.Millisecond {
		t.Fatalf("zh-dir-2 timing = %v..%v, want 5.1s..8.1s", at5500.Cue.Start, at5500.Cue.End)
	}
	if err := checkViewConsistency(cat, at5500); err != nil {
		t.Fatal(err)
	}

	// Pause again, switch language while paused, then resume.
	p.Pause()
	clock.Advance(time.Minute)
	if _, err := p.Switch(context.Background(), Criteria{Language: "en"}); err != nil {
		t.Fatal(err)
	}
	pausedEN := p.Snapshot()
	if pausedEN.Position != 5500*time.Millisecond {
		t.Fatalf("second pause position = %v, want 5.5s", pausedEN.Position)
	}
	if pausedEN.Cue == nil || pausedEN.Cue.ID != "en-2" {
		t.Fatalf("English cue while paused = %+v", pausedEN.Cue)
	}

	p.Resume()
	clock.Advance(500 * time.Millisecond)
	at6s := p.Snapshot()
	if at6s.Position != 6*time.Second {
		t.Fatalf("position after second resume = %v, want 6s", at6s.Position)
	}
	if at6s.Decision.Track.ID != "en-main" || at6s.Cue == nil || at6s.Cue.ID != "en-2" {
		t.Fatalf("post-resume state = track %+v cue %+v", at6s.Decision.Track, at6s.Cue)
	}
	if err := checkViewConsistency(cat, at6s); err != nil {
		t.Fatal(err)
	}
}

// TestAcceptance_SwitchWhilePlaying_KeepsCanonicalPosition verifies switching
// does not restart or reset playback and the new schedule is used at once.
func TestAcceptance_SwitchWhilePlaying_KeepsCanonicalPosition(t *testing.T) {
	cat, sel := fixtureSelector()
	clock := NewFakeClock(time.Unix(1_700_000_000, 0))
	p := NewPlayer("live", sel, WithClock(clock))

	p.Play()
	clock.Advance(8500 * time.Millisecond)
	result, err := p.Switch(context.Background(), Criteria{Language: "zh"})
	if err != nil {
		t.Fatal(err)
	}
	if result.View.Position != 8500*time.Millisecond {
		t.Fatalf("switch reset position to %v, want 8.5s", result.View.Position)
	}
	if result.View.Cue == nil || result.View.Cue.ID != "zh-main-3" {
		t.Fatalf("cue at 8.5s = %+v, want zh-main-3", result.View.Cue)
	}

	clock.Advance(500 * time.Millisecond)
	snap := p.Snapshot()
	if snap.Position != 9*time.Second {
		t.Fatalf("position after switch = %v, want 9s", snap.Position)
	}
	if err := checkViewConsistency(cat, snap); err != nil {
		t.Fatal(err)
	}
}
