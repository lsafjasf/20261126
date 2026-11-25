package audiopkg

import "sort"

// Reorder stably reorders the tracks of p according to less.
// It only permutes the slice: whole Track values are swapped, so no
// field of any track is modified. The default track, roles, languages
// and accessibility flags all stay attached to their original track.
func (p *Package) Reorder(less func(a, b *Track) bool) {
	sort.SliceStable(p.Tracks, func(i, j int) bool {
		return less(&p.Tracks[i], &p.Tracks[j])
	})
}

// SortCanonical orders tracks by language, then main tracks first,
// then accessibility tracks last, keeping the relative order of
// otherwise-equal tracks. Fields are never altered, only positions.
func (p *Package) SortCanonical() {
	p.Reorder(func(a, b *Track) bool {
		if a.Language != b.Language {
			return a.Language < b.Language
		}
		return rank(a) < rank(b)
	})
}

func rank(t *Track) int {
	switch {
	case t.IsAccessibilityTrack():
		return 2
	case t.Role == RoleMain:
		return 0
	default:
		return 1
	}
}
