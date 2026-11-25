package captionhub

// ConnectionState is the authoritative visible caption state for one viewer
// connection. It is safe to JSON-encode and send to operators or clients.
type ConnectionState struct {
	ConnectionID   string      `json:"connection_id"`
	Mode           Mode        `json:"mode"`
	Status         Status      `json:"status"`
	Reason         string      `json:"reason,omitempty"`
	ResyncRequired bool        `json:"resync_required"`
	ProgramID      string      `json:"program_id,omitempty"`
	SegmentID      string      `json:"segment_id,omitempty"`
	SessionID      string      `json:"session_id,omitempty"`
	Epoch          uint64      `json:"epoch,omitempty"`
	TimelineID     string      `json:"timeline_id,omitempty"`
	MediaTimeMS    *Millis     `json:"media_time_ms,omitempty"`
	VisibleCues    []Cue       `json:"visible_cues"`
	ActiveCue      *Cue        `json:"active_cue,omitempty"`
	RejectedCues   int64       `json:"rejected_cues"`
	Revision       uint64      `json:"revision"`
	Checkpoint     *Checkpoint `json:"checkpoint,omitempty"`
}

type connState struct {
	id                  string
	mode                Mode
	sessionID           string
	epoch               uint64
	hasPosition         bool
	mediaTime           Millis
	status              Status
	reason              string
	resync              bool
	visible             []Cue
	rejectedCues        int64
	revision            uint64
	attemptedCheckpoint *Checkpoint
}

func (c *connState) checkpoint() *Checkpoint {
	if c.sessionID == "" || c.epoch == 0 {
		return nil
	}
	cp := &Checkpoint{
		SessionID: c.sessionID,
		Epoch:     c.epoch,
		Mode:      c.mode,
	}
	if c.hasPosition {
		cp.MediaTimeMS = c.mediaTime
	}
	return cp
}

func (h *Hub) viewLocked(c *connState) ConnectionState {
	view := ConnectionState{
		ConnectionID:   c.id,
		Mode:           c.mode,
		Status:         c.status,
		Reason:         c.reason,
		ResyncRequired: c.resync,
		SessionID:      c.sessionID,
		Epoch:          c.epoch,
		VisibleCues:    append([]Cue{}, c.visible...),
		RejectedCues:   c.rejectedCues,
		Revision:       c.revision,
	}
	if view.VisibleCues == nil {
		view.VisibleCues = []Cue{}
	}
	if c.hasPosition {
		pos := c.mediaTime
		view.MediaTimeMS = &pos
	}
	if len(c.visible) > 0 {
		active := c.visible[0]
		view.ActiveCue = &active
	}
	if sess, ok := h.sessions[c.sessionID]; ok {
		view.ProgramID = sess.segment.ProgramID
		view.SegmentID = sess.segment.SegmentID
		view.TimelineID = timelineID(sess.id, c.epoch)
	}
	if cp := c.checkpoint(); cp != nil {
		if sess, ok := h.sessions[c.sessionID]; ok {
			cp.ProgramID = sess.segment.ProgramID
			cp.SegmentID = sess.segment.SegmentID
		}
		view.Checkpoint = cp
	}
	if c.attemptedCheckpoint != nil {
		attempted := *c.attemptedCheckpoint
		view.Checkpoint = &attempted
	}
	return view
}

func (h *Hub) refreshLocked(c *connState, reason string) {
	c.reason = reason
	if c.resync {
		c.status = StatusResyncRequired
		c.visible = []Cue{}
		return
	}
	sess, ok := h.sessions[c.sessionID]
	if !ok || c.epoch == 0 {
		c.status = StatusNoSession
		c.visible = []Cue{}
		return
	}
	ep, ok := sess.epochs[c.epoch]
	if !ok {
		c.status = StatusResyncRequired
		c.resync = true
		c.reason = ReasonUnknownEpoch
		c.visible = []Cue{}
		return
	}
	if !c.hasPosition {
		c.status = StatusAwaitingPosition
		c.visible = []Cue{}
		return
	}
	c.visible = visibleCues(ep.cues, c.mediaTime)
	if len(c.visible) > 0 {
		c.status = StatusVisible
	} else {
		c.status = StatusIdle
	}
}

func clearForTimelineChange(c *connState, reason string) {
	c.epoch = 0
	c.hasPosition = false
	c.mediaTime = 0
	c.visible = []Cue{}
	c.resync = false
	c.status = StatusReset
	c.reason = reason
	c.attemptedCheckpoint = nil
}
