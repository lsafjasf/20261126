package subselect

import (
	"sort"
	"strings"
)

// Reason explains why a particular track became the selection.
type Reason string

const (
	// ReasonTrackID: exact TrackID request matched.
	ReasonTrackID Reason = "track_id"
	// ReasonLanguage: requested language/version matched.
	ReasonLanguage Reason = "language"
	// ReasonVersionFallback: requested version missing, another same-language version chosen.
	ReasonVersionFallback Reason = "version_fallback"
	// ReasonFallbackLanguage: requested language missing, configured fallback language chosen.
	ReasonFallbackLanguage Reason = "fallback_language"
	// ReasonLoadFallback: higher-ranked candidates failed to load.
	ReasonLoadFallback Reason = "load_fallback"
	// ReasonDefault: no language match; the Default track was used.
	ReasonDefault Reason = "default"
	// ReasonFirst: no default; the first usable track by ID was used.
	ReasonFirst Reason = "first"
	// ReasonOff: no usable track at all; subtitles are off.
	ReasonOff Reason = "off"
)

// SelectorConfig tunes the deterministic fallback chain.
type SelectorConfig struct {
	// FallbackLanguages are tried in order when Criteria.Language has no
	// usable track, e.g. []string{"en"}.
	FallbackLanguages []string
}

// Selector is the stateless subtitle resolver. It is safe to share between
// browsers/goroutines: it never stores "the current track". Per-browser
// current selection lives in Player.
type Selector struct {
	catalog *Catalog
	cfg     SelectorConfig
}

// NewSelector builds a Selector over a shared catalog.
func NewSelector(catalog *Catalog, cfg SelectorConfig) *Selector {
	cp := SelectorConfig{FallbackLanguages: append([]string(nil), cfg.FallbackLanguages...)}
	return &Selector{catalog: catalog, cfg: cp}
}

// Catalog returns the bound catalog.
func (s *Selector) Catalog() *Catalog { return s.catalog }

// Decision is an immutable resolution result.
type Decision struct {
	// Criteria is the normalized request.
	Criteria Criteria `json:"criteria"`

	// Track is the preferred track; nil means subtitles off.
	Track *Track `json:"track,omitempty"`

	// Reason explains Track.
	Reason Reason `json:"reason"`

	// Candidates is the full ordered fallback chain of track IDs that the
	// player should attempt to load. Empty means off.
	Candidates []string `json:"candidates"`

	// Variants lists every usable same-language version ID for the requested
	// language, making ambiguous multi-version catalogs observable.
	Variants []string `json:"variants,omitempty"`

	LanguageMissing       bool `json:"language_missing,omitempty"`
	VersionMissing        bool `json:"version_missing,omitempty"`
	RequestedTrackMissing bool `json:"requested_track_missing,omitempty"`
}

// Select resolves criteria to one preferred track plus an ordered load chain.
func (s *Selector) Select(c Criteria) Decision {
	c = c.withDefaults()
	d := Decision{Criteria: c, Reason: ReasonOff}
	if s == nil || s.catalog == nil {
		return d
	}

	all := s.catalog.Tracks()
	usable := make([]Track, 0, len(all))
	for _, t := range all {
		if !t.Unavailable {
			usable = append(usable, t)
		}
	}

	// 1. Exact track id wins outright.
	if c.TrackID != "" {
		if t, ok := s.catalog.Get(c.TrackID); ok && !t.Unavailable {
			d.Track = &t
			d.Reason = ReasonTrackID
			d.Candidates = s.chainFrom(t, usable, c)
			return d
		}
		d.RequestedTrackMissing = true
	}

	// 2. Requested language group.
	var langGroup []Track
	if c.Language != "" {
		langGroup = matchLanguage(usable, c.Language)
		if len(langGroup) == 0 {
			d.LanguageMissing = true
		} else {
			sortVariants(langGroup, c)
			d.Variants = ids(langGroup)
			chosen := langGroup[0]
			reason := ReasonLanguage
			if c.Version != "" && !equalFold(chosen.Version, c.Version) {
				d.VersionMissing = true
				reason = ReasonVersionFallback
			}
			d.Track = &chosen
			d.Reason = reason
			d.Candidates = s.chainFrom(chosen, usable, c)
			return d
		}
		if c.Version != "" {
			d.VersionMissing = true
		}
	}

	// 3. Configured fallback languages, but only when a language was
	// actually requested and could not be satisfied.
	if c.Language != "" {
		for _, lang := range s.cfg.FallbackLanguages {
			group := matchLanguage(usable, lang)
			if len(group) == 0 {
				continue
			}
			sortVariants(group, c)
			chosen := group[0]
			d.Track = &chosen
			d.Reason = ReasonFallbackLanguage
			d.Candidates = s.chainFrom(chosen, usable, c)
			return d
		}
	}

	// 4. Default track.
	for _, t := range usable {
		if t.Default {
			chosen := t
			d.Track = &chosen
			d.Reason = ReasonDefault
			d.Candidates = s.chainFrom(chosen, usable, c)
			return d
		}
	}

	// 5. First usable track (usable is already ID-sorted from Tracks()).
	if len(usable) > 0 {
		chosen := usable[0]
		d.Track = &chosen
		d.Reason = ReasonFirst
		d.Candidates = s.chainFrom(chosen, usable, c)
		return d
	}

	return d
}

// chainFrom builds the de-duplicated ordered load chain: chosen track first,
// then its same-language siblings, then every other usable track. Loaders
// and missing-track fallbacks walk this chain.
func (s *Selector) chainFrom(chosen Track, usable []Track, c Criteria) []string {
	ordered := make([]Track, 0, len(usable))
	seen := map[string]bool{chosen.ID: true}
	ordered = append(ordered, chosen)

	siblings := matchLanguage(usable, chosen.Language)
	sortVariants(siblings, c)
	for _, t := range siblings {
		if !seen[t.ID] {
			seen[t.ID] = true
			ordered = append(ordered, t)
		}
	}

	rest := append([]Track(nil), usable...)
	sort.SliceStable(rest, func(i, j int) bool {
		if rest[i].Default != rest[j].Default {
			return rest[i].Default
		}
		if rest[i].Ranking != rest[j].Ranking {
			return rest[i].Ranking > rest[j].Ranking
		}
		return rest[i].ID < rest[j].ID
	})
	for _, t := range rest {
		if !seen[t.ID] {
			ordered = append(ordered, t)
		}
	}
	return ids(ordered)
}

func ids(ts []Track) []string {
	out := make([]string, 0, len(ts))
	for _, t := range ts {
		out = append(out, t.ID)
	}
	return out
}

// sortVariants orders same-language versions by exact version, role
// preferences, default flag, manual ranking and finally ID. The final ID
// tie-break makes language-only selection deterministic.
func sortVariants(ts []Track, c Criteria) {
	sort.SliceStable(ts, func(i, j int) bool {
		ti, tj := ts[i], ts[j]
		if vi, vj := versionScore(ti, c), versionScore(tj, c); vi != vj {
			return vi > vj
		}
		if pi, pj := preferenceScore(ti, c), preferenceScore(tj, c); pi != pj {
			return pi > pj
		}
		if ti.Default != tj.Default {
			return ti.Default
		}
		if ti.Ranking != tj.Ranking {
			return ti.Ranking > tj.Ranking
		}
		return ti.ID < tj.ID
	})
}

func versionScore(t Track, c Criteria) int {
	if c.Version != "" && equalFold(t.Version, c.Version) {
		return 1000
	}
	return 0
}

func preferenceScore(t Track, c Criteria) int {
	score := 0
	if c.Kind != "" && t.Kind == c.Kind {
		score += 40
	}
	switch c.Forced {
	case Yes:
		if t.Forced {
			score += 20
		}
	case No:
		if !t.Forced {
			score += 20
		}
	case Any:
		// A forced track is a specialist role; never let it become the
		// generic language default merely by ID tie-break.
		if t.Forced {
			score -= 100
		}
	}
	switch c.HearingImpaired {
	case Yes:
		if t.HearingImpaired {
			score += 20
		}
	case No:
		if !t.HearingImpaired {
			score += 20
		}
	case Any:
		// Prefer regular subtitles over SDH/captions unless asked for.
		if t.HearingImpaired {
			score -= 10
		}
	}
	return score
}

// matchLanguage returns tracks whose tag equals lang or shares its primary
// subtag ("zh-CN" matches "zh"). Order is preserved from the input.
func matchLanguage(tracks []Track, lang string) []Track {
	want := normalizeLang(lang)
	if want == "" {
		return nil
	}
	wantBase := baseLang(want)
	var exact, base []Track
	for _, t := range tracks {
		got := normalizeLang(t.Language)
		if got == "" {
			continue
		}
		if got == want {
			exact = append(exact, t)
			continue
		}
		if baseLang(got) == wantBase {
			base = append(base, t)
		}
	}
	// Exact tags first, then primary-subtag matches.
	return append(append([]Track{}, exact...), base...)
}

func normalizeLang(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func baseLang(tag string) string {
	tag = normalizeLang(tag)
	if i := strings.IndexAny(tag, "-_"); i >= 0 {
		return tag[:i]
	}
	return tag
}

func equalFold(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}
