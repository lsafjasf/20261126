package anchor

import "sort"

func applyRemovedDecision(piece *Piece, d Decision) {
	switch d.Action {
	case ActionDrop:
		piece.Status = StatusMapped
		piece.Dropped = true
	case ActionCustom:
		piece.Status = StatusMapped
		piece.DstStart = d.DstStart
		piece.DstEnd = d.DstEnd
	case ActionSegment:
		// The chosen segment no longer covers this source range, so the
		// decision is stale and the piece stays unresolved.
	}
}

func applyAmbiguousDecision(piece *Piece, d Decision, segs []Segment, res *Result, key string) {
	switch d.Action {
	case ActionSegment:
		for _, s := range segs {
			if s.ID == d.SegmentID {
				piece.Status = StatusMapped
				mapThrough(piece, s)
				return
			}
		}
		// No matching candidate: leave the piece ambiguous and report stale.
		res.Stale = append(res.Stale, StaleDecision{
			Key:    key,
			Reason: "segment " + d.SegmentID + " is no longer a candidate for this fragment",
		})
	case ActionCustom:
		piece.Status = StatusMapped
		piece.DstStart = d.DstStart
		piece.DstEnd = d.DstEnd
	case ActionDrop:
		piece.Status = StatusMapped
		piece.Dropped = true
	}
}

func (p *Project) collectStale(livePieceKeys map[string]bool, legacyEventIDs map[string]bool,
	segByID map[string]Segment) []StaleDecision {

	var stale []StaleDecision
	for key, d := range p.Decisions {
		switch {
		case !livePieceKeys[key]:
			reason := "fragment no longer exists after the latest edit"
			if d.Action == ActionSegment && segByID[d.SegmentID] == (Segment{}) {
				reason = "referenced segment " + d.SegmentID + " was removed from the EDL"
			}
			stale = append(stale, StaleDecision{Key: key, Reason: reason})
		case d.Action == ActionSegment && !p.segmentCoversPiece(key, d.SegmentID):
			stale = append(stale, StaleDecision{
				Key:    key,
				Reason: "segment " + d.SegmentID + " no longer covers this fragment",
			})
		}
	}
	for eventID := range p.AnchorDecisions {
		if legacyEventIDs[eventID] {
			continue
		}
		// The decision is stale only when the event disappeared from the
		// project entirely. A legacy event still present continues to be
		// anchored by this decision; an event that gained its own anchor
		// simply overrides it.
		if !p.eventExists(eventID) {
			stale = append(stale, StaleDecision{
				Key:    "anchor:" + eventID,
				Reason: "event no longer exists",
			})
		}
	}
	sort.Slice(stale, func(i, j int) bool { return stale[i].Key < stale[j].Key })
	return stale
}

func (p *Project) eventExists(id string) bool {
	for _, ev := range p.Events {
		if ev.ID == id {
			return true
		}
	}
	return false
}

// segmentCoversPiece reports whether segmentID still covers the source range
// encoded in a stable piece key, after re-deriving the fragment from the EDL.
func (p *Project) segmentCoversPiece(key, segmentID string) bool {
	eventID, anchor, ok := parsePieceKey(key)
	if !ok {
		return false
	}
	for _, ev := range p.Events {
		if ev.ID != eventID {
			continue
		}
		effective := p.effectiveAnchor(ev)
		if effective == nil || effective.MediaID != anchor.MediaID {
			return false
		}
		for _, fr := range splitAnchor(anchor, p.Edl) {
			if fr.srcStart != anchor.Start || fr.srcEnd != anchor.End {
				continue
			}
			for _, s := range fr.segments {
				if s.ID == segmentID {
					return true
				}
			}
		}
	}
	return false
}

func sortResult(res *Result) {
	sort.Slice(res.Mapped, func(i, j int) bool {
		return res.Mapped[i].EventID < res.Mapped[j].EventID
	})
	for _, me := range res.Mapped {
		sort.Slice(me.Pieces, func(i, j int) bool {
			if me.Pieces[i].SrcStart != me.Pieces[j].SrcStart {
				return me.Pieces[i].SrcStart < me.Pieces[j].SrcStart
			}
			return me.Pieces[i].Key < me.Pieces[j].Key
		})
	}
	sort.Slice(res.Unresolved, func(i, j int) bool {
		if res.Unresolved[i].EventID != res.Unresolved[j].EventID {
			return res.Unresolved[i].EventID < res.Unresolved[j].EventID
		}
		return res.Unresolved[i].Key < res.Unresolved[j].Key
	})
}
