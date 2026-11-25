package captionhub

import (
	"fmt"
	"sort"
	"strings"
)

type epoch struct {
	number  uint64
	reason  string
	cues    []Cue
	cueIDs  map[string]struct{}
	nextSeq int64
}

type session struct {
	id              string
	segment         Segment
	epochs          map[uint64]*epoch
	epochOrder      []uint64
	currentEpoch    uint64
	discontinuities int
	sealed          bool
}

func newSession(id string, seg Segment, epochNo uint64, reason string) *session {
	if reason == "" {
		reason = ReasonSegmentStart
	}
	return &session{
		id:           id,
		segment:      seg,
		epochs:       map[uint64]*epoch{epochNo: newEpoch(epochNo, reason)},
		epochOrder:   []uint64{epochNo},
		currentEpoch: epochNo,
	}
}

func newEpoch(number uint64, reason string) *epoch {
	return &epoch{
		number:  number,
		reason:  reason,
		cueIDs:  make(map[string]struct{}),
		nextSeq: 1,
	}
}

func (s *session) current() *epoch {
	return s.epochs[s.currentEpoch]
}

func (s *session) addEpoch(reason string) *epoch {
	s.currentEpoch++
	if reason == "" {
		reason = ReasonDiscontinuity
	}
	ep := newEpoch(s.currentEpoch, reason)
	s.epochs[s.currentEpoch] = ep
	s.epochOrder = append(s.epochOrder, s.currentEpoch)
	return ep
}

func (s *session) info() SessionInfo {
	ep := s.current()
	reason := ReasonSegmentStart
	if ep != nil {
		reason = ep.reason
	}
	return SessionInfo{
		SessionID:       s.id,
		ProgramID:       s.segment.ProgramID,
		SegmentID:       s.segment.SegmentID,
		Epoch:           s.currentEpoch,
		TimelineID:      timelineID(s.id, s.currentEpoch),
		Reason:          reason,
		Discontinuities: s.discontinuities,
	}
}

func timelineID(sessionID string, epochNo uint64) string {
	return fmt.Sprintf("%s#e%d", sessionID, epochNo)
}

func validateCue(cue Cue) error {
	if cue.StartMS < 0 {
		return fmt.Errorf("%w: start must be non-negative", ErrInvalidCue)
	}
	if cue.EndMS <= cue.StartMS {
		return fmt.Errorf("%w: end must be greater than start", ErrInvalidCue)
	}
	if strings.TrimSpace(cue.Text) == "" {
		return fmt.Errorf("%w: text is empty", ErrInvalidCue)
	}
	return nil
}

func (ep *epoch) insert(cue Cue) (Cue, error) {
	if err := validateCue(cue); err != nil {
		return Cue{}, err
	}
	if cue.ID == "" {
		cue.ID = fmt.Sprintf("cue-%d", ep.nextSeq)
	}
	if _, exists := ep.cueIDs[cue.ID]; exists {
		return Cue{}, fmt.Errorf("%w: %s", ErrDuplicateCue, cue.ID)
	}
	if cue.Sequence == 0 {
		cue.Sequence = ep.nextSeq
	}
	ep.nextSeq = max(ep.nextSeq, cue.Sequence+1)
	ep.cueIDs[cue.ID] = struct{}{}
	ep.cues = append(ep.cues, cue)
	sort.SliceStable(ep.cues, func(i, j int) bool {
		if ep.cues[i].StartMS != ep.cues[j].StartMS {
			return ep.cues[i].StartMS < ep.cues[j].StartMS
		}
		return ep.cues[i].Sequence < ep.cues[j].Sequence
	})
	return cue, nil
}

// visibleCues returns an independent, sorted copy of cues active at pos.
// Interval semantics are [StartMS, EndMS).
func visibleCues(cues []Cue, pos Millis) []Cue {
	var out []Cue
	for _, cue := range cues {
		if cue.visibleAt(pos) {
			out = append(out, cue)
		}
	}
	if out == nil {
		return []Cue{}
	}
	return out
}

// SelectVisible is a viewer-side helper that drops any cue from another
// session epoch before evaluating time intervals.
func SelectVisible(cues []Cue, cp Checkpoint, pos Millis) []Cue {
	scoped := make([]Cue, 0, len(cues))
	for _, cue := range cues {
		if cue.SessionID != cp.SessionID || cue.Epoch != cp.Epoch {
			continue
		}
		scoped = append(scoped, cue)
	}
	return visibleCues(scoped, pos)
}
