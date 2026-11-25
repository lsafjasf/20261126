package sdh

import (
	"fmt"
	"sort"
	"strings"
)

// Split parses cues into semantic layers and resolves overlaps using the
// default (unlimited lane) layout.
func Split(cues []Cue) *Project {
	return SplitWithOptions(cues, DefaultLayoutOptions())
}

// SplitWithOptions is Split with explicit overlap layout options.
func SplitWithOptions(cues []Cue, layoutOpts LayoutOptions) *Project {
	p := NewProject()
	// Copy so we can fill missing IDs without mutating the caller's slice.
	p.Cues = append([]Cue(nil), cues...)

	for i := range p.Cues {
		cue := &p.Cues[i]
		if cue.ID == "" {
			switch {
			case cue.Index != 0:
				cue.ID = fmt.Sprintf("%d", cue.Index)
			default:
				cue.ID = fmt.Sprintf("cue-%d", i+1)
			}
		}
		if cue.Index == 0 {
			cue.Index = i + 1
		}

		segments := ClassifyCue(*cue)
		p.Segments[cue.ID] = segments

		for _, seed := range buildEntrySeeds(segments) {
			seg := seed.seg
			if strings.TrimSpace(strings.Join(seed.texts, "")) == "" {
				continue
			}
			entry := &Entry{
				ID:        fmt.Sprintf("%s#%d", cue.ID, seed.segIndex),
				CueID:     cue.ID,
				CueIndex:  cue.Index,
				SegIndex:  seed.segIndex,
				Role:      seg.Role,
				Text:      strings.Join(seed.texts, "\n"),
				Speaker:   seg.Speaker,
				VoiceOver: seg.VoiceOver,
				OffScreen: seg.OffScreen,
				Start:     cue.Start,
				End:       cue.End,
				Group:     -1,
				Lane:      -1,
			}
			p.Entries = append(p.Entries, entry)

			layer := p.Layers[seg.Role]
			if layer == nil {
				layer = &Layer{Role: seg.Role}
				p.Layers[seg.Role] = layer
			}
			layer.Entries = append(layer.Entries, entry)
		}
	}

	entryLess := func(a, b *Entry) bool {
		if a.Start != b.Start {
			return a.Start < b.Start
		}
		if a.End != b.End {
			return a.End > b.End // longer cues first on ties
		}
		if roleOrder(a.Role) != roleOrder(b.Role) {
			return roleOrder(a.Role) < roleOrder(b.Role)
		}
		return a.ID < b.ID
	}
	sort.SliceStable(p.Entries, func(i, j int) bool {
		return entryLess(p.Entries[i], p.Entries[j])
	})

	for _, role := range Roles() {
		if layer := p.Layers[role]; layer != nil && len(layer.Entries) > 0 {
			sort.SliceStable(layer.Entries, func(i, j int) bool {
				return entryLess(layer.Entries[i], layer.Entries[j])
			})
			p.LayerOrder = append(p.LayerOrder, role)
		}
	}

	p.Layout = ResolveOverlaps(p.Entries, layoutOpts)
	return p
}

// entrySeed is one entry under construction. Vocal segments with the same
// role and speaker are merged (joined by a line break) so ordinary wrapped
// dialogue stays a single multi-line block instead of being lane-stacked.
type entrySeed struct {
	seg      Segment
	segIndex int
	texts    []string
}

func (s *entrySeed) key() string {
	return fmt.Sprintf("%s|%s|%t|%t", s.seg.Role, s.seg.Speaker, s.seg.VoiceOver, s.seg.OffScreen)
}

// buildEntrySeeds turns ordered classified segments into entry seeds.
// Non-vocal segments (speaker labels, descriptions) always become their own
// seed; vocal segments sharing role/speaker/delivery are merged.
func buildEntrySeeds(segments []Segment) []*entrySeed {
	var seeds []*entrySeed
	open := map[string]*entrySeed{}

	for i, seg := range segments {
		if strings.TrimSpace(seg.Text) == "" {
			continue
		}
		if !seg.Role.Vocal() {
			seeds = append(seeds, &entrySeed{seg: seg, segIndex: i, texts: []string{seg.Text}})
			continue
		}
		probe := &entrySeed{seg: seg}
		if existing := open[probe.key()]; existing != nil {
			existing.texts = append(existing.texts, seg.Text)
			continue
		}
		seed := &entrySeed{seg: seg, segIndex: i, texts: []string{seg.Text}}
		seeds = append(seeds, seed)
		open[seed.key()] = seed
	}
	return seeds
}
