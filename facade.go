package safereflow

// At returns a safe-area span covering timestamp ts. Spans are time-sorted.
// When several spans overlap at ts, the conservative effective rectangle is
// their intersection; use SlicesOver for the merged result.
func (t *SafeTrack) At(ts float64) (SafeSpan, bool) {
	if t == nil {
		return SafeSpan{}, false
	}
	for _, sp := range t.spans {
		if sp.Contains(ts) {
			return sp, true
		}
	}
	return SafeSpan{}, false
}

// AutoReflow runs the full safe-reflow pipeline: analyze dynamic safe areas,
// then apply only recommended auto-eligible plans. Low-confidence migrations
// are reported but never applied. The returned document is a deep copy.
func AutoReflow(doc *CueDocument, track *SafeTrack) (*Report, *CueDocument, *ApplyReport, error) {
	eng := NewEngine(doc, track)
	rep, err := eng.Analyze()
	if err != nil {
		return nil, nil, nil, err
	}
	newDoc, applied, err := eng.ApplyAuto(rep)
	if err != nil {
		return rep, nil, applied, err
	}
	return rep, newDoc, applied, nil
}
