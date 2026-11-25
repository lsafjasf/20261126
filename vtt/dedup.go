package vtt

// DedupReport summarizes a Deduplicate run.
type DedupReport struct {
	// Removed is the number of duplicate cues dropped.
	Removed int
	// Kept is the number of cues retained after deduplication.
	Kept int
	// RepeatedIDs counts identifiers occurring on more than one retained
	// cue. These are legal distinct segments, not duplicates.
	RepeatedIDs map[string]int
	// RemovedIDs lists the identifier of every removed cue, in removal
	// order, so callers can audit what happened.
	RemovedIDs []string
}

// Deduplicate drops cues whose full three-layer Identity (cue ID + time
// interval + text) was already seen. The first occurrence keeps its
// original position; non-cue blocks (STYLE/REGION/NOTE) are never touched.
//
// Cues that merely share an identifier but differ in timing or text are
// retained, which fixes the failure mode of treating cue IDs as unique
// keys. Anonymous cues (empty ID) are compared on timing and text alone.
func (d *Document) Deduplicate() *DedupReport {
	report := &DedupReport{RepeatedIDs: map[string]int{}}
	seen := map[Identity]struct{}{}
	idCounts := map[string]int{}

	kept := make([]Block, 0, len(d.Blocks))
	for _, block := range d.Blocks {
		cue, ok := block.(*Cue)
		if !ok {
			kept = append(kept, block)
			continue
		}
		identity := cue.Identity()
		if _, exists := seen[identity]; exists {
			report.Removed++
			report.RemovedIDs = append(report.RemovedIDs, cue.ID)
			continue
		}
		seen[identity] = struct{}{}
		kept = append(kept, cue)
		report.Kept++
		if cue.ID != "" {
			idCounts[cue.ID]++
		}
	}

	for id, count := range idCounts {
		if count > 1 {
			report.RepeatedIDs[id] = count
		}
	}
	d.Blocks = kept
	return report
}

// IsDuplicate reports whether two cues are genuine duplicates, i.e. equal
// on all three identity layers.
func IsDuplicate(a, b *Cue) bool {
	if a == nil || b == nil {
		return false
	}
	return a.Identity() == b.Identity()
}
