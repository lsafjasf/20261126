package cea708

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ErrNetworkDown is returned by a Source while the network is interrupted.
var ErrNetworkDown = errors.New("cea708: network interrupted")

// ErrSequenceGap means ingestion skipped a required sequence; the client must
// rebuild from a recovery point instead of applying disjoint data.
var ErrSequenceGap = errors.New("cea708: sequence gap")

// Seq is the monotonic, contiguous network sequence used for recovery.
type Seq uint64

// Item is one sequence-addressable ingestion unit. Packet holds one assembled
// DTVCC packet. Select optionally moves the visible service at this sequence
// (an out-of-band program/service switch). Reset marks a clean, self-contained
// recovery boundary (typically an IDR-aligned RST + DFx + DSW epoch).
type Item struct {
	Seq    Seq           `json:"seq"`
	PTS    time.Duration `json:"pts_ns"`
	Packet []byte        `json:"packet,omitempty"`
	Select ServiceID     `json:"select,omitempty"`
	Reset  bool          `json:"reset,omitempty"`
}

// Journal is an append-only, contiguous replay log.
type Journal struct {
	mu    sync.RWMutex
	items []Item
}

// NewJournal creates an empty journal.
func NewJournal() *Journal { return &Journal{} }

// Append records an item; sequences must stay contiguous starting at 1.
func (j *Journal) Append(it Item) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	want := Seq(len(j.items) + 1)
	if it.Seq != want {
		return fmt.Errorf("cea708: journal append expects seq %d, got %d", want, it.Seq)
	}
	j.items = append(j.items, it)
	return nil
}

// Items returns a copy of the log.
func (j *Journal) Items() []Item {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return append([]Item(nil), j.items...)
}

// From returns items with Seq >= seq.
func (j *Journal) From(seq Seq) []Item {
	j.mu.RLock()
	defer j.mu.RUnlock()
	var out []Item
	for _, it := range j.items {
		if it.Seq >= seq {
			out = append(out, it)
		}
	}
	return out
}

// Last returns the final logged item.
func (j *Journal) Last() (Item, bool) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	if len(j.items) == 0 {
		return Item{}, false
	}
	return j.items[len(j.items)-1], true
}

// RecoveryPoint is the durable resume token. With a Snapshot present, rebuild
// restores it and only replays Seq+1 onward; otherwise rebuild starts a fresh
// decoder at Boundary, the latest clean reset point.
type RecoveryPoint struct {
	Version  int              `json:"version"`
	Seq      Seq              `json:"seq"`
	PTS      time.Duration    `json:"pts_ns,omitempty"`
	Boundary Seq              `json:"boundary"`
	Snapshot *ManagerSnapshot `json:"snapshot,omitempty"`
}

// Token encodes the point as a URL-safe base64 JSON string.
func (p RecoveryPoint) Token() (string, error) {
	p.Version = SnapshotVersion
	raw, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// ParseRecoveryPoint decodes Token output.
func ParseRecoveryPoint(token string) (RecoveryPoint, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return RecoveryPoint{}, fmt.Errorf("cea708: bad recovery token: %w", err)
	}
	var p RecoveryPoint
	if err := json.Unmarshal(raw, &p); err != nil {
		return RecoveryPoint{}, fmt.Errorf("cea708: bad recovery payload: %w", err)
	}
	return p, nil
}

// Source is the network feed used by rebuild.
type Source interface {
	FetchFrom(seq Seq) ([]Item, error)
}

// MemSource is an in-memory scripted network source. Toggle Down to simulate
// an interruption.
type MemSource struct {
	Items []Item
	Down  bool
}

// FetchFrom returns all items with Seq >= seq in order.
func (s *MemSource) FetchFrom(seq Seq) ([]Item, error) {
	if s.Down {
		return nil, ErrNetworkDown
	}
	var out []Item
	for _, it := range s.Items {
		if it.Seq >= seq {
			out = append(out, it)
		}
	}
	return out, nil
}

// IngestReport describes what one Ingest call did.
type IngestReport struct {
	Seq       Seq
	Applied   bool
	Duplicate bool
	Warning   string
}

// RecoverySession ties a Manager to a Journal and a contiguous applied
// high-water mark. Redelivered/overlapping items are deduplicated so replay
// never draws a glyph twice.
type RecoverySession struct {
	Manager  *Manager
	Journal  *Journal
	Applied  Seq
	Boundary Seq
}

// NewRecoverySession wraps m.
func NewRecoverySession(m *Manager) *RecoverySession {
	return &RecoverySession{Manager: m, Journal: NewJournal()}
}

// Ingest applies one item exactly once. Items at or below Applied are reported
// as duplicates and skipped; a forward gap returns ErrSequenceGap.
func (rs *RecoverySession) Ingest(it Item) (IngestReport, error) {
	rep := IngestReport{Seq: it.Seq}
	if it.Seq <= rs.Applied {
		rep.Duplicate = true
		return rep, nil
	}
	if it.Seq != rs.Applied+1 {
		return rep, fmt.Errorf("%w: applied=%d got=%d", ErrSequenceGap, rs.Applied, it.Seq)
	}

	// Apply media first so the target service's latch is fresh, then perform
	// the selection: the resulting switch frame atomically shows the new
	// service content on a cleared canvas instead of a blank-then-draw pair.
	if len(it.Packet) > 0 {
		if err := rs.Manager.Ingest(uint64(it.Seq), it.Packet); err != nil {
			// Malformed command framing is non-fatal: state before the fault
			// is retained and the sequence still advances.
			rep.Warning = err.Error()
		}
	}
	if it.Select != NoService {
		rs.Manager.SelectService(it.Select)
	}

	rs.Applied = it.Seq
	if it.Reset {
		rs.Boundary = it.Seq
	}
	_ = rs.Journal.Append(it)
	rep.Applied = true
	return rep, nil
}

// Checkpoint produces a RecoveryPoint. WithSnapshot embeds every service
// screen so a later rebuild only needs suffix replay.
func (rs *RecoverySession) Checkpoint(withSnapshot bool) RecoveryPoint {
	cp := RecoveryPoint{Version: SnapshotVersion, Seq: rs.Applied, Boundary: rs.Boundary}
	if last, ok := rs.Journal.Last(); ok {
		cp.PTS = last.PTS
	}
	if withSnapshot {
		snap := rs.Manager.Snapshot()
		cp.Snapshot = &snap
	}
	return cp
}

// ReplayAll builds a Manager and applies items in order.
func ReplayAll(items []Item, opts ...Option) (*Manager, *RecoverySession, error) {
	m := NewManager(opts...)
	rs := NewRecoverySession(m)
	for _, it := range items {
		if _, err := rs.Ingest(it); err != nil {
			return m, rs, err
		}
	}
	return m, rs, nil
}

// RebuildReport summarizes a Rebuild run.
type RebuildReport struct {
	StartSeq     Seq  `json:"start_seq"`
	Applied      int  `json:"applied"`
	Skipped      int  `json:"skipped"`
	UsedSnapshot bool `json:"used_snapshot"`
	FinalSeq     Seq  `json:"final_seq"`
}

// Rebuild reconstructs manager state from rp against src. If rp carries a
// snapshot it is restored and only Seq+1..target is replayed; otherwise a fresh
// decoder replays from rp.Boundary (the clean CEA-708 reset/random-access
// point). target=0 replays everything the source returns.
func Rebuild(rp RecoveryPoint, src Source, target Seq, opts ...Option) (*Manager, RebuildReport, error) {
	rep := RebuildReport{}

	var m *Manager
	var fetchFrom Seq
	var base Seq
	if rp.Snapshot != nil {
		m = RestoreManager(*rp.Snapshot)
		base = rp.Seq
		fetchFrom = rp.Seq + 1
		rep.UsedSnapshot = true
	} else {
		m = NewManager(opts...)
		boundary := rp.Boundary
		if boundary == 0 {
			boundary = 1
		}
		base = boundary - 1
		fetchFrom = boundary
	}
	rep.StartSeq = fetchFrom

	items, err := src.FetchFrom(fetchFrom)
	if err != nil {
		return m, rep, err
	}

	rs := &RecoverySession{Manager: m, Journal: NewJournal(), Applied: base, Boundary: rp.Boundary}
	for _, it := range items {
		if target != 0 && it.Seq > target {
			break
		}
		r, err := rs.Ingest(it)
		if err != nil {
			return m, rep, err
		}
		if r.Applied {
			rep.Applied++
		}
		if r.Duplicate {
			rep.Skipped++
		}
	}
	rep.FinalSeq = rs.Applied
	return m, rep, nil
}
