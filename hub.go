package subselect

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"sync/atomic"
)

// Hub owns the per-browser Player sessions for one selector/catalog.
// The Hub map is shared but every Player has independent state, which is
// exactly the isolation the reported bug violated.
type Hub struct {
	selector *Selector
	loader   Loader
	clock    Clock

	mu       sync.Mutex
	sessions map[string]*Player
}

// HubOption configures a Hub.
type HubOption func(*Hub)

// WithHubLoader sets the loader used for new sessions.
func WithHubLoader(l Loader) HubOption {
	return func(h *Hub) { h.loader = l }
}

// WithHubClock sets the clock used for new sessions.
func WithHubClock(c Clock) HubOption {
	return func(h *Hub) { h.clock = c }
}

// NewHub creates an empty session hub.
func NewHub(selector *Selector, opts ...HubOption) *Hub {
	h := &Hub{
		selector: selector,
		loader:   StaticLoader{},
		clock:    SystemClock{},
		sessions: make(map[string]*Player),
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// Create mints a new isolated player with a random session id.
func (h *Hub) Create() *Player {
	id := newSessionID()
	return h.CreateWithID(id)
}

// CreateWithID registers a player under an externally supplied id.
func (h *Hub) CreateWithID(id string) *Player {
	p := NewPlayer(id, h.selector, WithLoader(h.loader), WithClock(h.clock))
	h.mu.Lock()
	h.sessions[id] = p
	h.mu.Unlock()
	return p
}

// Get returns the session player.
func (h *Hub) Get(id string) (*Player, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	p, ok := h.sessions[id]
	return p, ok
}

// Delete removes a session.
func (h *Hub) Delete(id string) {
	h.mu.Lock()
	delete(h.sessions, id)
	h.mu.Unlock()
}

// IDs returns all session ids.
func (h *Hub) IDs() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]string, 0, len(h.sessions))
	for id := range h.sessions {
		out = append(out, id)
	}
	return out
}

func newSessionID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("session-fallback-%d", fallbackCounter.Add(1))
	}
	return hex.EncodeToString(b[:])
}

var fallbackCounter atomic.Uint64
