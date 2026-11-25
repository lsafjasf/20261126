package captionhub

import (
	"fmt"
	"sort"
	"sync"
)

// Hub owns caption sessions and the visible state of every connection.
// All methods are safe for concurrent use.
type Hub struct {
	mu             sync.RWMutex
	idPrefix       string
	revision       uint64
	nextSessionNo  uint64
	sessions       map[string]*session
	sessionOrder   []string
	segmentIndex   map[Segment]string
	currentSession string
	connections    map[string]*connState
}

// Option configures a Hub.
type Option func(*Hub)

// WithIDPrefix sets the prefix used for generated session ids.
func WithIDPrefix(prefix string) Option {
	return func(h *Hub) {
		if prefix != "" {
			h.idPrefix = prefix
		}
	}
}

// NewHub creates an empty caption session hub.
func NewHub(opts ...Option) *Hub {
	h := &Hub{
		idPrefix:     "cs",
		sessions:     make(map[string]*session),
		segmentIndex: make(map[Segment]string),
		connections:  make(map[string]*connState),
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

func (h *Hub) bumpLocked() uint64 {
	h.revision++
	return h.revision
}

// Revision returns the current monotonically increasing hub revision.
func (h *Hub) Revision() uint64 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.revision
}

// BeginSegment starts the caption session for a new program segment.
// Live connections are moved to the new timeline and their old cues are
// cleared. Replay connections remain pinned to their recovered session.
func (h *Hub) BeginSegment(seg Segment) (SessionInfo, error) {
	if !seg.valid() {
		return SessionInfo{}, fmt.Errorf("%w: program_id and segment_id are required", ErrInvalidCheckpoint)
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	key := seg.key()
	if existingID, ok := h.segmentIndex[key]; ok {
		return SessionInfo{}, fmt.Errorf("%w: %s/%s -> %s", ErrSegmentExists, seg.ProgramID, seg.SegmentID, existingID)
	}

	h.nextSessionNo++
	sessionID := seg.SessionID
	if sessionID == "" {
		sessionID = fmt.Sprintf("%s-%d", h.idPrefix, h.nextSessionNo)
	}
	if _, exists := h.sessions[sessionID]; exists {
		return SessionInfo{}, fmt.Errorf("%w: %s", ErrSessionExists, sessionID)
	}

	if old, ok := h.sessions[h.currentSession]; ok {
		old.sealed = true
	}
	sess := newSession(sessionID, seg, 1, ReasonSegmentStart)
	h.sessions[sessionID] = sess
	h.sessionOrder = append(h.sessionOrder, sessionID)
	h.segmentIndex[key] = sessionID
	h.currentSession = sessionID
	h.bumpLocked()

	for _, conn := range h.connections {
		if conn.mode == ModeLive {
			conn.sessionID = sessionID
			clearForTimelineChange(conn, ReasonProgramSwitch)
			conn.epoch = 1
			conn.revision = h.revision
		}
	}
	return sess.info(), nil
}

// SwitchProgram is a descriptive alias for beginning a new program segment.
func (h *Hub) SwitchProgram(programID, segmentID string) (SessionInfo, error) {
	return h.BeginSegment(Segment{ProgramID: programID, SegmentID: segmentID})
}

// Discontinuity starts a new timeline epoch in the current program segment.
// Live connections drop cues from the previous epoch and wait for a position
// on the reset timeline.
func (h *Hub) Discontinuity(reason string) (SessionInfo, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	sess, err := h.currentLocked()
	if err != nil {
		return SessionInfo{}, err
	}
	if reason == "" {
		reason = ReasonDiscontinuity
	}
	sess.addEpoch(reason)
	sess.discontinuities++
	h.bumpLocked()

	for _, conn := range h.connections {
		if conn.mode == ModeLive && conn.sessionID == sess.id {
			conn.sessionID = sess.id
			clearForTimelineChange(conn, ReasonDiscontinuity)
			conn.epoch = sess.currentEpoch
			conn.revision = h.revision
		}
	}
	return sess.info(), nil
}

func (h *Hub) currentLocked() (*session, error) {
	if h.currentSession == "" {
		return nil, ErrNoSession
	}
	sess, ok := h.sessions[h.currentSession]
	if !ok {
		return nil, ErrNoSession
	}
	return sess, nil
}

// CurrentSession returns information about the active live session.
func (h *Hub) CurrentSession() (SessionInfo, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	sess, err := h.currentLocked()
	if err != nil {
		return SessionInfo{}, err
	}
	return sess.info(), nil
}

// Connect opens a live connection. It has no media position until the player
// reports one, so no cue is guessed.
func (h *Hub) Connect(id string) (ConnectionState, error) {
	if id == "" {
		return ConnectionState{}, fmt.Errorf("%w: empty connection id", ErrInvalidCheckpoint)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, exists := h.connections[id]; exists {
		return ConnectionState{}, fmt.Errorf("%w: %s", ErrConnectionExists, id)
	}

	conn := &connState{
		id:       id,
		mode:     ModeLive,
		visible:  []Cue{},
		revision: h.revision + 1,
		status:   StatusNoSession,
		reason:   ReasonConnected,
	}
	if sess, err := h.currentLocked(); err == nil {
		conn.sessionID = sess.id
		conn.epoch = sess.currentEpoch
		conn.status = StatusAwaitingPosition
	}
	h.bumpLocked()
	conn.revision = h.revision
	h.connections[id] = conn
	return h.viewLocked(conn), nil
}

// Disconnect removes a connection.
func (h *Hub) Disconnect(id string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.connections[id]; !ok {
		return fmt.Errorf("%w: %s", ErrConnectionNotFound, id)
	}
	delete(h.connections, id)
	h.bumpLocked()
	return nil
}

// ForgetSession removes an archived, non-current session. Replay connections
// pinned to it are moved to resync_required instead of silently keeping cues.
func (h *Hub) ForgetSession(sessionID string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.sessions[sessionID]; !ok {
		return fmt.Errorf("%w: %s", ErrSessionNotFound, sessionID)
	}
	if h.currentSession == sessionID {
		return fmt.Errorf("%w: %s", ErrSessionActive, sessionID)
	}

	delete(h.sessions, sessionID)
	for key, id := range h.segmentIndex {
		if id == sessionID {
			delete(h.segmentIndex, key)
		}
	}
	for i, id := range h.sessionOrder {
		if id == sessionID {
			h.sessionOrder = append(h.sessionOrder[:i], h.sessionOrder[i+1:]...)
			break
		}
	}

	h.bumpLocked()
	for _, conn := range h.connections {
		if conn.sessionID != sessionID {
			continue
		}
		conn.visible = []Cue{}
		conn.resync = true
		conn.status = StatusResyncRequired
		conn.reason = ReasonStaleCheckpoint
		conn.revision = h.revision
	}
	return nil
}

// State returns the current visible caption state for one connection.
func (h *Hub) State(id string) (ConnectionState, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	conn, ok := h.connections[id]
	if !ok {
		return ConnectionState{}, fmt.Errorf("%w: %s", ErrConnectionNotFound, id)
	}
	return h.viewLocked(conn), nil
}

// States returns all connection states sorted by connection id.
func (h *Hub) States() []ConnectionState {
	h.mu.RLock()
	defer h.mu.RUnlock()

	ids := make([]string, 0, len(h.connections))
	for id := range h.connections {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]ConnectionState, 0, len(ids))
	for _, id := range ids {
		out = append(out, h.viewLocked(h.connections[id]))
	}
	return out
}
