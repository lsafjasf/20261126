package anchor

import "sort"

// fragment is one maximal sub-range of an event anchor together with every
// EDL segment covering that exact sub-range.
type fragment struct {
	mediaID  string
	srcStart Ticks
	srcEnd   Ticks
	segments []Segment
}

// splitAnchor cuts an anchor interval at every EDL segment boundary that lies
// inside it, producing deterministic source-ordered fragments. A reused range
// (same footage placed on the timeline several times) yields one fragment
// with multiple segments, which is what makes the ambiguity explicit.
func splitAnchor(a Anchor, edl []Segment) []fragment {
	boundSet := map[Ticks]bool{a.Start: true, a.End: true}
	var covering []Segment
	for _, s := range edl {
		if s.MediaID != a.MediaID || s.SrcEnd <= a.Start || s.SrcStart >= a.End {
			continue
		}
		covering = append(covering, s)
		if s.SrcStart > a.Start && s.SrcStart < a.End {
			boundSet[s.SrcStart] = true
		}
		if s.SrcEnd > a.Start && s.SrcEnd < a.End {
			boundSet[s.SrcEnd] = true
		}
	}

	bounds := make([]Ticks, 0, len(boundSet))
	for b := range boundSet {
		bounds = append(bounds, b)
	}
	sort.Slice(bounds, func(i, j int) bool { return bounds[i] < bounds[j] })

	var out []fragment
	for i := 0; i+1 < len(bounds); i++ {
		lo, hi := bounds[i], bounds[i+1]
		if hi <= lo {
			continue
		}
		var segs []Segment
		for _, s := range covering {
			if s.SrcStart <= lo && s.SrcEnd >= hi {
				segs = append(segs, s)
			}
		}
		sort.Slice(segs, func(i, j int) bool {
			if segs[i].DstStart != segs[j].DstStart {
				return segs[i].DstStart < segs[j].DstStart
			}
			return segs[i].ID < segs[j].ID
		})
		out = append(out, fragment{
			mediaID:  a.MediaID,
			srcStart: lo,
			srcEnd:   hi,
			segments: segs,
		})
	}
	return out
}
