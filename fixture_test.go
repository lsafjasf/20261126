package subselect

import (
	"fmt"
	"time"
)

// fixtureCatalog builds tracks with deliberately different time bases and
// offsets. If a switch swaps text but forgets Rebase, checkViewConsistency
// fails because every cue window is independently recomputed here.
func fixtureCatalog() *Catalog {
	tracks := []Track{
		{
			ID: "zh-main", Language: "zh-CN", Version: "theatrical",
			Title: "简体中文（公映）", Kind: KindSubtitles, Default: true, Ranking: 10,
			TimeBase: TimeBase{Timescale: 1000},
			Cues: []Cue{
				{ID: "zh-main-1", Start: 1000, End: 3000, Text: "你好"},
				{ID: "zh-main-2", Start: 4000, End: 7000, Text: "我们走吧"},
				{ID: "zh-main-3", Start: 8000, End: 9500, Text: "再见"},
			},
		},
		{
			ID: "zh-director", Language: "zh", Version: "director-cut",
			Title: "中文导演剪辑版", Kind: KindSubtitles, Ranking: 5,
			// Same language, different cut: offset +500ms and shifted cues.
			TimeBase: TimeBase{Timescale: 1000, Offset: 500 * time.Millisecond},
			Cues: []Cue{
				{ID: "zh-dir-1", Start: 1200, End: 3200, Text: "你好（导剪）"},
				{ID: "zh-dir-2", Start: 4600, End: 7600, Text: "走，我们离开这"},
				{ID: "zh-dir-3", Start: 8200, End: 9800, Text: "后会有期"},
			},
		},
		{
			ID: "en-main", Language: "en", Version: "theatrical",
			Title: "English Theatrical", Kind: KindSubtitles,
			// 90 kHz media time base.
			TimeBase: TimeBase{Timescale: 90000},
			Cues: []Cue{
				{ID: "en-1", Start: 90000, End: 270000, Text: "Hello"},
				{ID: "en-2", Start: 405000, End: 675000, Text: "Let's go"}, // 4.5s..7.5s
				{ID: "en-3", Start: 720000, End: 855000, Text: "Goodbye"},
			},
		},
		{
			ID: "en-sdh", Language: "en-US", Version: "sdh",
			Title: "English SDH", Kind: KindCaptions, HearingImpaired: true,
			TimeBase: TimeBase{Timescale: 48000, Offset: 250 * time.Millisecond},
			Cues: []Cue{
				{ID: "en-sdh-1", Start: 36000, End: 132000, Text: "[DOOR CREAKS] Hello"},
				{ID: "en-sdh-2", Start: 216000, End: 348000, Text: "[FOOTSTEPS] Let's go"},
			},
		},
		{
			ID: "en-forced", Language: "en", Version: "forced",
			Kind: KindForced, Forced: true,
			TimeBase: TimeBase{Timescale: 1000},
			Cues: []Cue{
				{ID: "en-forced-1", Start: 5000, End: 6000, Text: "[Foreign speech]"},
			},
		},
		{
			ID: "ko-disabled", Language: "ko",
			Unavailable: true,
			TimeBase:    TimeBase{Timescale: 1000},
		},
	}
	cat, err := NewCatalog(tracks...)
	if err != nil {
		panic(err)
	}
	return cat
}

func fixtureSelector(fallbacks ...string) (*Catalog, *Selector) {
	cat := fixtureCatalog()
	return cat, NewSelector(cat, SelectorConfig{FallbackLanguages: fallbacks})
}

// checkViewConsistency verifies the core invariant: track identity, rebased
// timing and active-cue text all come from the same selected track/time base.
func checkViewConsistency(cat *Catalog, v *View) error {
	if v == nil {
		return fmt.Errorf("nil view")
	}

	if v.Off {
		if v.Schedule != nil {
			return fmt.Errorf("off view carries a schedule")
		}
		if v.Decision.Track != nil {
			return fmt.Errorf("off view carries decision track %q", v.Decision.Track.ID)
		}
		if v.Cue != nil {
			return fmt.Errorf("off view carries a cue")
		}
		return nil
	}

	track := v.Decision.Track
	if track == nil {
		// Initial / in-flight state with no committed selection yet.
		if v.Schedule == nil && v.Cue == nil {
			return nil
		}
		return fmt.Errorf("non-off view has nil decision track")
	}
	if v.Schedule == nil {
		return fmt.Errorf("track %q view has nil schedule", track.ID)
	}
	if v.Schedule.TrackID != track.ID {
		return fmt.Errorf("schedule track %q != decision track %q (mixed shared state)",
			v.Schedule.TrackID, track.ID)
	}
	if v.Schedule.Language != track.Language {
		return fmt.Errorf("schedule language %q != track language %q",
			v.Schedule.Language, track.Language)
	}

	src, ok := cat.Get(track.ID)
	if !ok {
		return fmt.Errorf("selected track %q missing from catalog", track.ID)
	}
	byID := make(map[string]Cue, len(src.Cues))
	for _, c := range src.Cues {
		byID[c.ID] = c
	}

	for _, got := range v.Schedule.Cues {
		want, ok := byID[got.ID]
		if !ok {
			return fmt.Errorf("schedule contains unknown cue %q", got.ID)
		}
		if got.TrackID != track.ID || got.Language != track.Language {
			return fmt.Errorf("cue %q identity %q/%q does not match view %q/%q",
				got.ID, got.TrackID, got.Language, track.ID, track.Language)
		}
		wantStart := src.TimeBase.ToDuration(want.Start)
		wantEnd := src.TimeBase.ToDuration(want.End)
		if got.Start != wantStart || got.End != wantEnd {
			return fmt.Errorf(
				"cue %q timing not rebased from track %q time base: got [%v,%v), want [%v,%v) (old time base leak)",
				got.ID, track.ID, got.Start, got.End, wantStart, wantEnd)
		}
		if got.Text != want.Text {
			return fmt.Errorf("cue %q text mismatch: got %q want %q", got.ID, got.Text, want.Text)
		}
	}

	if v.Cue != nil {
		cue := v.Cue
		if !cue.Contains(v.Position) {
			return fmt.Errorf("active cue %q window [%v,%v) does not contain position %v",
				cue.ID, cue.Start, cue.End, v.Position)
		}
		want, ok := byID[cue.ID]
		if !ok {
			return fmt.Errorf("active cue %q not in source track %q", cue.ID, track.ID)
		}
		if cue.Text != want.Text {
			return fmt.Errorf("active cue %q text %q is not track %q text %q (new language/old state mix)",
				cue.ID, cue.Text, track.ID, want.Text)
		}
		if cue.Start != src.TimeBase.ToDuration(want.Start) ||
			cue.End != src.TimeBase.ToDuration(want.End) {
			return fmt.Errorf("active cue %q uses stale timing %v..%v for new text",
				cue.ID, cue.Start, cue.End)
		}
	}

	return nil
}
