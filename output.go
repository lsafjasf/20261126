package captionhub

import (
	"encoding/json"
	"io"
	"sort"
)

// Snapshot is a point-in-time view of sessions and all connection states.
type Snapshot struct {
	Revision       uint64            `json:"revision"`
	CurrentSession *SessionInfo      `json:"current_session,omitempty"`
	Sessions       []SessionInfo     `json:"sessions"`
	Connections    []ConnectionState `json:"connections"`
}

// Snapshot returns a deterministic, JSON-serializable snapshot.
func (h *Hub) Snapshot() Snapshot {
	h.mu.RLock()
	defer h.mu.RUnlock()

	snapshot := Snapshot{
		Revision:    h.revision,
		Sessions:    make([]SessionInfo, 0, len(h.sessionOrder)),
		Connections: make([]ConnectionState, 0, len(h.connections)),
	}
	for _, id := range h.sessionOrder {
		if sess, ok := h.sessions[id]; ok {
			snapshot.Sessions = append(snapshot.Sessions, sess.info())
		}
	}
	if current, ok := h.sessions[h.currentSession]; ok {
		info := current.info()
		snapshot.CurrentSession = &info
	}

	ids := make([]string, 0, len(h.connections))
	for id := range h.connections {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		snapshot.Connections = append(snapshot.Connections, h.viewLocked(h.connections[id]))
	}
	return snapshot
}

// WriteJSON writes the full snapshot as indented JSON.
func (h *Hub) WriteJSON(w io.Writer) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(h.Snapshot())
}

// WriteStatesJSON writes only the per-connection states as a JSON array.
func (h *Hub) WriteStatesJSON(w io.Writer) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(h.States())
}
