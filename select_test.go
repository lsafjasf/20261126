package subselect

import (
	"errors"
	"testing"
)

func TestSelect_ExactTrackIDWins(t *testing.T) {
	_, sel := fixtureSelector("en")
	d := sel.Select(Criteria{TrackID: "zh-director", Language: "en"})
	if d.Reason != ReasonTrackID || d.Track == nil || d.Track.ID != "zh-director" {
		t.Fatalf("got reason=%s track=%v, want track_id/zh-director", d.Reason, d.Track)
	}
	if len(d.Candidates) == 0 || d.Candidates[0] != "zh-director" {
		t.Fatalf("candidate chain must start with exact id, got %v", d.Candidates)
	}
}

func TestSelect_MissingTrackIDMarksFlagAndFallsBack(t *testing.T) {
	_, sel := fixtureSelector()
	d := sel.Select(Criteria{TrackID: "does-not-exist", Language: "en"})
	if !d.RequestedTrackMissing {
		t.Fatal("expected RequestedTrackMissing")
	}
	if d.Track == nil || d.Track.ID != "en-main" {
		t.Fatalf("expected language fallback en-main, got %v", d.Track)
	}
}

func TestSelect_SameLanguageMultipleVersions(t *testing.T) {
	_, sel := fixtureSelector()

	// Language-only is deterministic and picks the Default version.
	first := sel.Select(Criteria{Language: "zh"}).Track.ID
	for i := 0; i < 20; i++ {
		if got := sel.Select(Criteria{Language: "zh-CN"}).Track.ID; got != first {
			t.Fatalf("non-deterministic language-only choice: %s vs %s", first, got)
		}
	}
	if first != "zh-main" {
		t.Fatalf("default version = %q, want zh-main", first)
	}

	d := sel.Select(Criteria{Language: "zh", Version: "director-cut"})
	if d.Track.ID != "zh-director" || d.Reason != ReasonLanguage {
		t.Fatalf("version selection got %q/%s", d.Track.ID, d.Reason)
	}
	if len(d.Variants) != 2 {
		t.Fatalf("expected 2 zh variants, got %v", d.Variants)
	}

	// Unknown version falls back to the best same-language variant and says so.
	d = sel.Select(Criteria{Language: "zh", Version: "imax-remaster"})
	if d.Track.ID != "zh-main" || d.Reason != ReasonVersionFallback || !d.VersionMissing {
		t.Fatalf("version fallback got track=%v reason=%s missing=%v", d.Track, d.Reason, d.VersionMissing)
	}
}

func TestSelect_HearingImpairedPreferencePicksSDH(t *testing.T) {
	_, sel := fixtureSelector()
	d := sel.Select(Criteria{Language: "en", HearingImpaired: Yes})
	if d.Track.ID != "en-sdh" {
		t.Fatalf("HI preference picked %q, want en-sdh", d.Track.ID)
	}

	d = sel.Select(Criteria{Language: "en", HearingImpaired: No})
	if d.Track.ID != "en-main" {
		t.Fatalf("non-HI preference picked %q, want en-main", d.Track.ID)
	}
}

func TestSelect_MissingLanguageFallbackChain(t *testing.T) {
	cat, sel := fixtureSelector("en")

	// ko exists but is unavailable: language reported missing, then en fallback.
	d := sel.Select(Criteria{Language: "ko"})
	if !d.LanguageMissing {
		t.Fatal("unavailable ko must count as language missing")
	}
	if d.Reason != ReasonFallbackLanguage || d.Track.ID != "en-main" {
		t.Fatalf("got %s/%v, want fallback_language/en-main", d.Reason, d.Track)
	}

	// ja does not exist at all: same fallback.
	d = sel.Select(Criteria{Language: "ja"})
	if d.Reason != ReasonFallbackLanguage || d.Track.Language != "en" {
		t.Fatalf("ja fallback = %s/%v", d.Reason, d.Track)
	}

	// No configured fallback: catalog Default wins.
	_, noFallback := fixtureSelector()
	d = noFallback.Select(Criteria{Language: "ja"})
	if d.Reason != ReasonDefault || d.Track.ID != "zh-main" {
		t.Fatalf("default fallback = %s/%v", d.Reason, d.Track)
	}

	// Empty catalog => off.
	empty, err := NewCatalog()
	if err != nil {
		t.Fatal(err)
	}
	d = NewSelector(empty, SelectorConfig{}).Select(Criteria{Language: "en"})
	if d.Reason != ReasonOff || d.Track != nil || len(d.Candidates) != 0 {
		t.Fatalf("empty catalog must resolve off, got %+v", d)
	}

	// Catalog with one non-default track => first.
	one, err := NewCatalog(Track{ID: "only", Language: "fr", TimeBase: TimeBase{Timescale: 1000}})
	if err != nil {
		t.Fatal(err)
	}
	d = NewSelector(one, SelectorConfig{}).Select(Criteria{Language: "de"})
	if d.Reason != ReasonFirst || d.Track.ID != "only" {
		t.Fatalf("first fallback = %s/%v", d.Reason, d.Track)
	}

	_ = cat
}

func TestSelect_CandidateChainIncludesSiblings(t *testing.T) {
	_, sel := fixtureSelector()
	d := sel.Select(Criteria{Language: "zh", Version: "director-cut"})
	if len(d.Candidates) < 3 {
		t.Fatalf("chain should include chosen + siblings + rest, got %v", d.Candidates)
	}
	if d.Candidates[0] != "zh-director" || d.Candidates[1] != "zh-main" {
		t.Fatalf("same-language siblings must precede other tracks: %v", d.Candidates)
	}
}

func TestNewCatalog_RejectsDuplicateAndBadTimeBase(t *testing.T) {
	if _, err := NewCatalog(
		Track{ID: "a", TimeBase: TimeBase{Timescale: 1000}},
		Track{ID: "a", TimeBase: TimeBase{Timescale: 1000}},
	); err == nil {
		t.Fatal("expected duplicate id error")
	}
	if _, err := NewCatalog(Track{ID: "b"}); !errors.Is(err, ErrInvalidTimeBase) {
		t.Fatalf("expected ErrInvalidTimeBase, got %v", err)
	}
}
