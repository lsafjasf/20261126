package anchor

import "sort"

// Unresolved describes one fragment or legacy event that still needs a human.
type Unresolved struct {
	EventID    string      `json:"event_id"`
	Kind       EventKind   `json:"kind"`
	Key        string      `json:"key,omitempty"`
	Status     PieceStatus `json:"status,omitempty"`
	MediaID    string      `json:"media_id,omitempty"`
	SrcStart   Ticks       `json:"src_start_ticks,omitempty"`
	SrcEnd     Ticks       `json:"src_end_ticks,omitempty"`
	Candidates []Candidate `json:"candidates,omitempty"`
	Legacy     bool        `json:"legacy,omitempty"`
}

// StaleDecision reports a stored decision that no longer matches the current
// EDL/events. The decision is kept on disk (never silently deleted) but is not
// applied, so the user can inspect or remove it explicitly.
type StaleDecision struct {
	Key    string `json:"key"`
	Reason string `json:"reason"`
}

// Result is the deterministic output of one mapping pass.
type Result struct {
	Mapped     []MappedEvent   `json:"mapped"`
	Unresolved []Unresolved    `json:"unresolved"`
	Stale      []StaleDecision `json:"stale_decisions"`
}

type segRef struct {
	seg Segment
}

// Map rebuilds every event against the EDL. It is a pure function of the
// project contents: calling it repeatedly, including after reload, yields
// identical output.
func (p *Project) Map() (*Result, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}

	segByID := make(map[string]Segment, len(p.Edl))
	for _, s := range p.Edl {
		segByID[s.ID] = s
	}

	res := &Result{}
	livePieceKeys := map[string]bool{}
	legacyEventIDs := map[string]bool{}

	for _, ev := range p.Events {
		anchor := p.effectiveAnchor(ev)
		if anchor == nil {
			legacyEventIDs[ev.ID] = true
			res.Unresolved = append(res.Unresolved, Unresolved{
				EventID: ev.ID,
				Kind:    ev.Kind,
				Key:     AnchorKey(ev.ID),
				Legacy:  true,
			})
			continue
		}

		me := MappedEvent{EventID: ev.ID, Kind: ev.Kind}
		for _, fr := range splitAnchor(*anchor, p.Edl) {
			key := PieceKey(ev.ID, Anchor{MediaID: fr.mediaID, Start: fr.srcStart, End: fr.srcEnd})
			livePieceKeys[key] = true
			piece := p.buildPiece(key, fr, segByID, res)
			me.Pieces = append(me.Pieces, piece)
			if piece.Status != StatusMapped {
				res.Unresolved = append(res.Unresolved, Unresolved{
					EventID:    ev.ID,
					Kind:       ev.Kind,
					Key:        key,
					Status:     piece.Status,
					MediaID:    piece.MediaID,
					SrcStart:   piece.SrcStart,
					SrcEnd:     piece.SrcEnd,
					Candidates: piece.Candidates,
				})
			}
		}
		res.Mapped = append(res.Mapped, me)
	}

	res.Stale = p.collectStale(livePieceKeys, legacyEventIDs, segByID)
	sortResult(res)
	return res, nil
}

func (p *Project) effectiveAnchor(ev Event) *Anchor {
	if ev.Anchor != nil {
		return ev.Anchor
	}
	if a, ok := p.AnchorDecisions[AnchorKey(ev.ID)]; ok {
		anchor := Anchor{MediaID: a.MediaID, Start: a.Start, End: a.End}
		return &anchor
	}
	return nil
}

func (p *Project) buildPiece(key string, fr fragment, segByID map[string]Segment, res *Result) Piece {
	piece := Piece{
		Key:      key,
		MediaID:  fr.mediaID,
		SrcStart: fr.srcStart,
		SrcEnd:   fr.srcEnd,
	}
	switch len(fr.segments) {
	case 0:
		piece.Status = StatusRemoved
		if d, ok := p.Decisions[key]; ok {
			applyRemovedDecision(&piece, d)
		}
	case 1:
		piece.Status = StatusMapped
		mapThrough(&piece, fr.segments[0])
	default:
		piece.Status = StatusAmbiguous
		piece.Candidates = buildCandidates(fr.segments, fr.srcStart, fr.srcEnd)
		if d, ok := p.Decisions[key]; ok {
			applyAmbiguousDecision(&piece, d, fr.segments, res, key)
		}
	}
	return piece
}

func mapThrough(piece *Piece, s Segment) {
	offset := piece.SrcStart - s.SrcStart
	length := piece.SrcEnd - piece.SrcStart
	piece.SegmentID = s.ID
	piece.DstStart = s.DstStart + offset
	piece.DstEnd = piece.DstStart + length
}

func buildCandidates(segs []Segment, srcStart, srcEnd Ticks) []Candidate {
	out := make([]Candidate, 0, len(segs))
	for _, s := range segs {
		offset := srcStart - s.SrcStart
		length := srcEnd - srcStart
		start := s.DstStart + offset
		out = append(out, Candidate{
			SegmentID: s.ID,
			DstStart:  start,
			DstEnd:    start + length,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].DstStart != out[j].DstStart {
			return out[i].DstStart < out[j].DstStart
		}
		return out[i].SegmentID < out[j].SegmentID
	})
	return out
}
