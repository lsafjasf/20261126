package anchor

import (
	"fmt"
	"sort"
)

// unresolvedIndex lets mutation helpers verify user input against the current
// mapping instead of accepting keys blindly.
type unresolvedIndex struct {
	pieces map[string]Unresolved
	legacy map[string]Unresolved
}

func (p *Project) indexUnresolved() (*unresolvedIndex, error) {
	res, err := p.Map()
	if err != nil {
		return nil, err
	}
	idx := &unresolvedIndex{
		pieces: map[string]Unresolved{},
		legacy: map[string]Unresolved{},
	}
	for _, u := range res.Unresolved {
		if u.Legacy {
			idx.legacy[u.EventID] = u
		} else {
			idx.pieces[u.Key] = u
		}
	}
	return idx, nil
}

// SetSegmentDecision records a manual choice of one EDL segment for an
// ambiguous fragment.
func (p *Project) SetSegmentDecision(key, segmentID, reason string) error {
	idx, err := p.indexUnresolved()
	if err != nil {
		return err
	}
	u, ok := idx.pieces[key]
	if !ok {
		return fmt.Errorf("no unresolved fragment with key %q", key)
	}
	if u.Status != StatusAmbiguous {
		return fmt.Errorf("fragment %q is %s; segment choice only applies to ambiguous fragments", key, u.Status)
	}
	for _, c := range u.Candidates {
		if c.SegmentID == segmentID {
			p.putDecision(key, Decision{Action: ActionSegment, SegmentID: segmentID, Reason: reason})
			return nil
		}
	}
	return fmt.Errorf("segment %q is not a candidate of fragment %q", segmentID, key)
}

// SetCustomDecision places a fragment at an explicit timeline interval. It is
// valid for ambiguous fragments and for fragments whose footage was removed.
func (p *Project) SetCustomDecision(key string, start, end Ticks, reason string) error {
	idx, err := p.indexUnresolved()
	if err != nil {
		return err
	}
	_, ok := idx.pieces[key]
	if !ok {
		return fmt.Errorf("no unresolved fragment with key %q", key)
	}
	if end <= start {
		return fmt.Errorf("custom interval must be non-empty")
	}
	p.putDecision(key, Decision{Action: ActionCustom, DstStart: start, DstEnd: end, Reason: reason})
	return nil
}

// SetDropDecision marks a removed or ambiguous fragment as intentionally
// dropped.
func (p *Project) SetDropDecision(key, reason string) error {
	idx, err := p.indexUnresolved()
	if err != nil {
		return err
	}
	if _, ok := idx.pieces[key]; !ok {
		return fmt.Errorf("no unresolved fragment with key %q", key)
	}
	p.putDecision(key, Decision{Action: ActionDrop, Reason: reason})
	return nil
}

// SetAnchorDecision anchors a legacy event that has no source anchor.
func (p *Project) SetAnchorDecision(eventID, mediaID string, start, end Ticks, reason string) error {
	idx, err := p.indexUnresolved()
	if err != nil {
		return err
	}
	if _, ok := idx.legacy[eventID]; !ok {
		return fmt.Errorf("event %q is not an unanchored legacy event", eventID)
	}
	decision := AnchorDecision{MediaID: mediaID, Start: start, End: end, Reason: reason}
	if err := validateAnchor(p.mediaIndex(), Anchor{MediaID: mediaID, Start: start, End: end}); err != nil {
		return fmt.Errorf("anchor decision: %w", err)
	}
	if p.AnchorDecisions == nil {
		p.AnchorDecisions = map[string]AnchorDecision{}
	}
	p.AnchorDecisions[AnchorKey(eventID)] = decision
	return nil
}

// RemoveDecision deletes a stored fragment decision so the fragment becomes
// unresolved again.
func (p *Project) RemoveDecision(key string) error {
	if _, ok := p.Decisions[key]; !ok {
		return fmt.Errorf("no decision with key %q", key)
	}
	delete(p.Decisions, key)
	if len(p.Decisions) == 0 {
		p.Decisions = nil
	}
	return nil
}

// RemoveAnchorDecision deletes a stored manual anchor decision.
func (p *Project) RemoveAnchorDecision(eventID string) error {
	key := AnchorKey(eventID)
	if _, ok := p.AnchorDecisions[key]; !ok {
		return fmt.Errorf("no anchor decision for event %q", eventID)
	}
	delete(p.AnchorDecisions, key)
	if len(p.AnchorDecisions) == 0 {
		p.AnchorDecisions = nil
	}
	return nil
}

func (p *Project) putDecision(key string, d Decision) {
	if p.Decisions == nil {
		p.Decisions = map[string]Decision{}
	}
	p.Decisions[key] = d
}

func (p *Project) mediaIndex() map[string]Media {
	out := make(map[string]Media, len(p.Media))
	for _, m := range p.Media {
		out[m.ID] = m
	}
	return out
}

// SortedDecisionKeys returns decision keys in deterministic order for UIs.
func (p *Project) SortedDecisionKeys() []string {
	keys := make([]string, 0, len(p.Decisions))
	for k := range p.Decisions {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
