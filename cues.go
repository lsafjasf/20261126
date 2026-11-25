package captionhub

import "fmt"

// AddCue stores a cue in its session epoch.
//
// A zero SessionID addresses the current live session; a zero Epoch addresses
// that session's current epoch. Explicit writes to a non-current epoch are
// rejected so an in-flight old cue cannot be reintroduced after reset.
func (h *Hub) AddCue(cue Cue) (Cue, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.addCueLocked(cue)
}

func (h *Hub) addCueLocked(cue Cue) (Cue, error) {
	if cue.SessionID == "" {
		sess, err := h.currentLocked()
		if err != nil {
			return Cue{}, err
		}
		cue.SessionID = sess.id
		if cue.Epoch == 0 {
			cue.Epoch = sess.currentEpoch
		}
	}

	sess, ok := h.sessions[cue.SessionID]
	if !ok {
		return Cue{}, fmt.Errorf("%w: %s", ErrSessionNotFound, cue.SessionID)
	}
	if sess.sealed {
		return Cue{}, fmt.Errorf("%w: %s", ErrStaleSession, sess.id)
	}
	if cue.Epoch == 0 {
		cue.Epoch = sess.currentEpoch
	}
	ep, ok := sess.epochs[cue.Epoch]
	if !ok {
		return Cue{}, fmt.Errorf("%w: %s epoch %d", ErrEpochNotFound, sess.id, cue.Epoch)
	}
	if cue.Epoch != sess.currentEpoch {
		return Cue{}, fmt.Errorf("%w: %s#e%d is sealed; current is e%d", ErrStaleEpoch, sess.id, cue.Epoch, sess.currentEpoch)
	}

	inserted, err := ep.insert(cue)
	if err != nil {
		return Cue{}, err
	}
	h.bumpLocked()

	for _, conn := range h.connections {
		if conn.sessionID == inserted.SessionID && conn.epoch == inserted.Epoch {
			h.refreshLocked(conn, conn.reason)
			conn.revision = h.revision
		}
	}
	return inserted, nil
}

// DeliverCue simulates sending a cue to one viewer connection. A cue whose
// session epoch does not match the connection checkpoint is counted and
// rejected without changing visible captions.
func (h *Hub) DeliverCue(connectionID string, cue Cue) (ConnectionState, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	conn, ok := h.connections[connectionID]
	if !ok {
		return ConnectionState{}, fmt.Errorf("%w: %s", ErrConnectionNotFound, connectionID)
	}
	if cue.SessionID == "" {
		cue.SessionID = conn.sessionID
	}
	if cue.Epoch == 0 {
		cue.Epoch = conn.epoch
	}
	if conn.sessionID == "" || conn.epoch == 0 || conn.resync ||
		conn.sessionID != cue.SessionID || conn.epoch != cue.Epoch {
		conn.rejectedCues++
		h.bumpLocked()
		conn.revision = h.revision
		staleErr := error(ErrStaleEpoch)
		if cue.SessionID != conn.sessionID {
			staleErr = ErrStaleSession
		}
		return h.viewLocked(conn), fmt.Errorf("%w: cue %s targets %s#e%d, connection is on %s#e%d",
			staleErr, cue.ID, cue.SessionID, cue.Epoch, conn.sessionID, conn.epoch)
	}

	if _, err := h.addCueLocked(cue); err != nil {
		return h.viewLocked(conn), err
	}
	return h.viewLocked(conn), nil
}

// Cues returns a copy of all cues in a checkpoint's epoch.
func (h *Hub) Cues(cp Checkpoint) ([]Cue, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	sess, ok := h.sessions[cp.SessionID]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrSessionNotFound, cp.SessionID)
	}
	ep, ok := sess.epochs[cp.Epoch]
	if !ok {
		return nil, fmt.Errorf("%w: %s#e%d", ErrEpochNotFound, cp.SessionID, cp.Epoch)
	}
	return append([]Cue{}, ep.cues...), nil
}
