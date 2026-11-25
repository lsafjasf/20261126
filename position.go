package captionhub

import "fmt"

// ConnectAt creates a live connection at a known live position. This is the
// late-viewer path: only cues active at pos are returned; expired cues are not
// replayed.
func (h *Hub) ConnectAt(id string, pos Millis) (ConnectionState, error) {
	if pos < 0 {
		return ConnectionState{}, fmt.Errorf("%w: position must be non-negative", ErrInvalidCheckpoint)
	}
	view, err := h.Connect(id)
	if err != nil {
		return view, err
	}
	return h.ReportPosition(id, pos)
}

// ReportPosition records the player's current session-local media time and
// recomputes visible cues. Seek and ordinary timeupdate share this path; moving
// backwards can reveal a cue again and moving forwards skips expired cues.
func (h *Hub) ReportPosition(id string, pos Millis) (ConnectionState, error) {
	return h.reportPosition(id, pos, ReasonTimeUpdate)
}

// Seek records an explicit playback-position recovery or seek.
func (h *Hub) Seek(id string, pos Millis) (ConnectionState, error) {
	return h.reportPosition(id, pos, ReasonSeek)
}

func (h *Hub) reportPosition(id string, pos Millis, reason string) (ConnectionState, error) {
	if pos < 0 {
		return ConnectionState{}, fmt.Errorf("%w: position must be non-negative", ErrInvalidCheckpoint)
	}
	h.mu.Lock()
	defer h.mu.Unlock()

	conn, ok := h.connections[id]
	if !ok {
		return ConnectionState{}, fmt.Errorf("%w: %s", ErrConnectionNotFound, id)
	}
	if conn.resync {
		view := h.viewLocked(conn)
		return view, fmt.Errorf("%w: %s", ErrResyncRequired, conn.reason)
	}
	if conn.sessionID == "" || conn.epoch == 0 {
		sess, err := h.currentLocked()
		if err != nil {
			return h.viewLocked(conn), err
		}
		conn.sessionID = sess.id
		conn.epoch = sess.currentEpoch
	}
	if _, ok := h.sessions[conn.sessionID]; !ok {
		conn.resync = true
		conn.status = StatusResyncRequired
		conn.reason = ReasonStaleCheckpoint
		h.bumpLocked()
		conn.revision = h.revision
		return h.viewLocked(conn), fmt.Errorf("%w: %s", ErrSessionNotFound, conn.sessionID)
	}

	conn.hasPosition = true
	conn.mediaTime = pos
	h.bumpLocked()
	h.refreshLocked(conn, reason)
	conn.revision = h.revision
	return h.viewLocked(conn), nil
}

// RecoverPosition reconnects a viewer from a persisted checkpoint.
//
// In ModeReplay, an available old session epoch is restored exactly. In
// ModeLive, a checkpoint from a rotated program segment asks for a new live
// resync because its media time belongs to the discarded timeline.
func (h *Hub) RecoverPosition(id string, cp Checkpoint) (ConnectionState, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	conn := h.connections[id]
	if conn == nil {
		conn = &connState{id: id, visible: []Cue{}}
		h.connections[id] = conn
	}
	if cp.Mode == "" {
		cp.Mode = ModeReplay
	}
	conn.mode = cp.Mode

	sess, epochNo, err := h.resolveCheckpointLocked(cp)
	if err != nil {
		conn.sessionID = cp.SessionID
		conn.epoch = cp.Epoch
		conn.hasPosition = false
		conn.mediaTime = cp.MediaTimeMS
		conn.visible = []Cue{}
		conn.resync = true
		conn.status = StatusResyncRequired
		conn.reason = ReasonStaleCheckpoint
		attempted := cp
		conn.attemptedCheckpoint = &attempted
		h.bumpLocked()
		conn.revision = h.revision
		view := h.viewLocked(conn)
		return view, err
	}

	conn.sessionID = sess.id
	conn.epoch = epochNo
	conn.mediaTime = cp.MediaTimeMS
	conn.hasPosition = true
	conn.resync = false
	conn.attemptedCheckpoint = nil
	h.bumpLocked()
	h.refreshLocked(conn, ReasonRecovered)
	conn.revision = h.revision

	if cp.Mode == ModeLive && (sess.id != h.currentSession || epochNo != sess.currentEpoch) {
		conn.resync = true
		conn.status = StatusReset
		conn.visible = []Cue{}
		conn.hasPosition = false
		conn.reason = ReasonProgramSwitch
		conn.sessionID = h.currentSession
		if current, err := h.currentLocked(); err == nil {
			conn.epoch = current.currentEpoch
		}
		h.bumpLocked()
		conn.revision = h.revision
		return h.viewLocked(conn), fmt.Errorf("%w: live checkpoint %s#e%d is not current", ErrStaleCheckpoint, sess.id, epochNo)
	}
	return h.viewLocked(conn), nil
}

// Recover is a shorter alias for RecoverPosition.
func (h *Hub) Recover(id string, cp Checkpoint) (ConnectionState, error) {
	return h.RecoverPosition(id, cp)
}

func (h *Hub) resolveCheckpointLocked(cp Checkpoint) (*session, uint64, error) {
	var sess *session

	if cp.SessionID != "" {
		found, ok := h.sessions[cp.SessionID]
		if !ok {
			return nil, 0, fmt.Errorf("%w: %s", ErrSessionNotFound, cp.SessionID)
		}
		sess = found
	} else if cp.ProgramID != "" && cp.SegmentID != "" {
		sessionID, ok := h.segmentIndex[Segment{ProgramID: cp.ProgramID, SegmentID: cp.SegmentID}]
		if !ok {
			return nil, 0, fmt.Errorf("%w: %s/%s", ErrSessionNotFound, cp.ProgramID, cp.SegmentID)
		}
		sess = h.sessions[sessionID]
	} else {
		return nil, 0, ErrInvalidCheckpoint
	}

	epochNo := cp.Epoch
	if epochNo == 0 {
		if len(sess.epochs) > 1 {
			return nil, 0, fmt.Errorf("%w: %s has %d epochs", ErrAmbiguousCheckpoint, sess.id, len(sess.epochs))
		}
		epochNo = sess.currentEpoch
	}
	if _, ok := sess.epochs[epochNo]; !ok {
		return nil, 0, fmt.Errorf("%w: %s#e%d", ErrEpochNotFound, sess.id, epochNo)
	}
	return sess, epochNo, nil
}

// ResyncToLive drops a stale checkpoint and binds the connection to the
// current live timeline at the player's newly reported position.
func (h *Hub) ResyncToLive(id string, pos Millis) (ConnectionState, error) {
	if pos < 0 {
		return ConnectionState{}, fmt.Errorf("%w: position must be non-negative", ErrInvalidCheckpoint)
	}
	h.mu.Lock()
	defer h.mu.Unlock()

	conn, ok := h.connections[id]
	if !ok {
		return ConnectionState{}, fmt.Errorf("%w: %s", ErrConnectionNotFound, id)
	}
	sess, err := h.currentLocked()
	if err != nil {
		return ConnectionState{}, err
	}
	conn.mode = ModeLive
	conn.sessionID = sess.id
	conn.epoch = sess.currentEpoch
	conn.resync = false
	conn.attemptedCheckpoint = nil
	conn.hasPosition = true
	conn.mediaTime = pos
	h.bumpLocked()
	h.refreshLocked(conn, ReasonLiveResync)
	conn.revision = h.revision
	return h.viewLocked(conn), nil
}

// FollowLive switches a replay/pinned connection back to live-follow mode.
func (h *Hub) FollowLive(id string) (ConnectionState, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	conn, ok := h.connections[id]
	if !ok {
		return ConnectionState{}, fmt.Errorf("%w: %s", ErrConnectionNotFound, id)
	}
	sess, err := h.currentLocked()
	if err != nil {
		return ConnectionState{}, err
	}
	conn.mode = ModeLive
	conn.sessionID = sess.id
	conn.epoch = sess.currentEpoch
	conn.hasPosition = false
	conn.mediaTime = 0
	conn.visible = []Cue{}
	conn.resync = false
	conn.attemptedCheckpoint = nil
	h.bumpLocked()
	h.refreshLocked(conn, ReasonProgramSwitch)
	conn.revision = h.revision
	return h.viewLocked(conn), nil
}
