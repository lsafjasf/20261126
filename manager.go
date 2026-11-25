package cea708

import "sync"

// ClearReason identifies why the composition canvas was explicitly cleared.
type ClearReason string

// ClearServiceSwitch is the anti-ghosting clear executed atomically as part of
// changing the selected service. It happens before recomposition of the newly
// selected service and is never presented as an intermediate frame.
const ClearServiceSwitch ClearReason = "service_switch"

// ClearRecord is an auditable clear-screen event.
type ClearRecord struct {
	FrameIndex int64       `json:"frame_index"`
	Reason     ClearReason `json:"reason"`
	From       ServiceID   `json:"from"`
	To         ServiceID   `json:"to"`
	StreamSeq  uint64      `json:"stream_seq"`
}

// Option configures a Manager.
type Option func(*Manager)

// WithCanvas sets the composition grid size.
func WithCanvas(cols, rows int) Option {
	return func(m *Manager) {
		if cols > 0 {
			m.cols = cols
		}
		if rows > 0 {
			m.rows = rows
		}
	}
}

// WithSelectedService establishes the initially visible service.
func WithSelectedService(id ServiceID) Option {
	return func(m *Manager) {
		if validService(id) {
			m.selected = id
			m.hasSel = true
		}
	}
}

func validService(id ServiceID) bool { return id >= 1 && id <= MaxService }

// Manager owns one independent Decoder per CEA-708 service and the visible
// presentation timeline. Selection changes never share or copy window state:
// switching latches the outgoing service, clears the compositor and presents
// only the incoming service's own screen.
type Manager struct {
	mu sync.RWMutex

	cols int
	rows int

	services map[ServiceID]*Decoder
	selected ServiceID
	hasSel   bool

	frames    []Frame
	clears    []ClearRecord
	frameN    int64
	streamSeq uint64
}

// NewManager creates an empty service manager.
func NewManager(opts ...Option) *Manager {
	m := &Manager{
		cols:     DefaultCols,
		rows:     DefaultRows,
		services: make(map[ServiceID]*Decoder),
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

func (m *Manager) decoderLocked(id ServiceID) *Decoder {
	d := m.services[id]
	if d == nil {
		d = NewDecoder(id)
		m.services[id] = d
	}
	return d
}

// Service returns the independent decoder for id, creating it on first use.
//
// Mutating the returned Decoder directly bypasses the Manager lock; prefer
// Update when the Manager is used concurrently.
func (m *Manager) Service(id ServiceID) *Decoder {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.decoderLocked(id)
}

// Services returns the service numbers that currently have state.
func (m *Manager) Services() []ServiceID {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]ServiceID, 0, len(m.services))
	for id := range m.services {
		out = append(out, id)
	}
	return out
}

// Selected returns the visible service, if any.
func (m *Manager) Selected() (ServiceID, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.selected, m.hasSel
}

// SetStreamSeq records the network sequence currently being applied; later
// presented frames carry it for recovery diagnostics.
func (m *Manager) SetStreamSeq(seq uint64) {
	m.mu.Lock()
	m.streamSeq = seq
	m.mu.Unlock()
}

// Update applies one atomic command batch to service id. After the batch the
// service latch is refreshed; if id is the selected service, exactly one frame
// is appended to the visible timeline. The returned frame reports whether it
// was presented.
func (m *Manager) Update(id ServiceID, batch func(*Decoder)) (Frame, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d := m.decoderLocked(id)
	batch(d)
	f := d.Frame(0, m.streamSeq, FramePresent, m.cols, m.rows)
	d.latchFrame(f)
	if m.hasSel && m.selected == id {
		m.frameN++
		f.Index = m.frameN
		d.latchFrame(f)
		m.frames = append(m.frames, f)
		return f, true
	}
	return f, false
}

// Present recomposes the selected service and appends a frame. It is useful
// after selection-free changes or to force a boundary.
func (m *Manager) Present() (Frame, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.hasSel {
		return Frame{}, false
	}
	d := m.decoderLocked(m.selected)
	m.frameN++
	f := d.Frame(m.frameN, m.streamSeq, FramePresent, m.cols, m.rows)
	d.latchFrame(f)
	m.frames = append(m.frames, f)
	return f, true
}

// SelectService changes the visible service using the safe switch rule:
//
//  1. the outgoing service's latest composition is already latched;
//  2. record and apply the service-boundary clear (fresh blank canvas);
//  3. atomically present the incoming service's own latched/recomposed screen.
//
// No intermediate cleared frame is published, so viewers never see a flash
// and never see the old service's glyphs composited over the new one.
func (m *Manager) SelectService(id ServiceID) Frame {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !validService(id) {
		return Frame{}
	}

	from := m.selected
	if m.hasSel && from == id {
		d := m.decoderLocked(id)
		f := d.Frame(m.frameN, m.streamSeq, FrameSwitch, m.cols, m.rows)
		return f
	}

	m.frameN++
	m.clears = append(m.clears, ClearRecord{
		FrameIndex: m.frameN,
		Reason:     ClearServiceSwitch,
		From:       ternaryService(m.hasSel, from, NoService),
		To:         id,
		StreamSeq:  m.streamSeq,
	})

	m.hasSel = true
	m.selected = id

	d := m.decoderLocked(id)
	f := d.Frame(m.frameN, m.streamSeq, FrameSwitch, m.cols, m.rows)
	f.Index = m.frameN
	d.latchFrame(f)
	m.frames = append(m.frames, f)
	return f
}

func ternaryService(ok bool, a, b ServiceID) ServiceID {
	if ok {
		return a
	}
	return b
}

// Frames returns a copy of the visible presentation timeline.
func (m *Manager) Frames() []Frame {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Frame, len(m.frames))
	copy(out, m.frames)
	return out
}

// ClearEvents returns the audited clear-screen events.
func (m *Manager) ClearEvents() []ClearRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]ClearRecord, len(m.clears))
	copy(out, m.clears)
	return out
}

// LastFrame returns the most recent presented frame.
func (m *Manager) LastFrame() (Frame, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.frames) == 0 {
		return Frame{}, false
	}
	return m.frames[len(m.frames)-1], true
}

// LatchedScreen returns the retained screen of an arbitrary service, even when
// it is not the selected/visible one.
func (m *Manager) LatchedScreen(id ServiceID) (Frame, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	d := m.services[id]
	if d == nil {
		return Frame{}, false
	}
	return d.LatchedFrame()
}
