package cea708

import "sort"

// SnapshotVersion is the current manager-state serialization format.
const SnapshotVersion = 1

// WindowSnapshot is the restorable state of one window.
type WindowSnapshot struct {
	ID      int          `json:"id"`
	Defined bool         `json:"defined"`
	Visible bool         `json:"visible"`
	Config  WindowConfig `json:"config"`
	// Text holds retained rows without trailing blanks.
	Text   []string `json:"text"`
	PenRow int      `json:"pen_row"`
	PenCol int      `json:"pen_col"`
}

// DecoderSnapshot is the independent per-service screen state.
type DecoderSnapshot struct {
	Service     ServiceID        `json:"service"`
	Current     int              `json:"current"`
	DelayTenths int              `json:"delay_tenths"`
	Windows     []WindowSnapshot `json:"windows"`
}

// ManagerSnapshot is a JSON-serializable checkpoint of every service screen.
type ManagerSnapshot struct {
	Version      int               `json:"version"`
	Cols         int               `json:"cols"`
	Rows         int               `json:"rows"`
	Selected     ServiceID         `json:"selected"`
	HasSelection bool              `json:"has_selection"`
	StreamSeq    uint64            `json:"stream_seq"`
	Services     []DecoderSnapshot `json:"services"`
}

// Snapshot captures all per-service window/screen state.
func (m *Manager) Snapshot() ManagerSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	snap := ManagerSnapshot{
		Version:      SnapshotVersion,
		Cols:         m.cols,
		Rows:         m.rows,
		Selected:     m.selected,
		HasSelection: m.hasSel,
		StreamSeq:    m.streamSeq,
	}
	ids := make([]int, 0, len(m.services))
	for id := range m.services {
		ids = append(ids, int(id))
	}
	sort.Ints(ids)
	for _, id := range ids {
		d := m.services[ServiceID(id)]
		ds := DecoderSnapshot{Service: d.id, Current: d.current, DelayTenths: d.DelayTenths}
		for i := range d.windows {
			w := &d.windows[i]
			ws := WindowSnapshot{
				ID: w.id, Defined: w.defined, Visible: w.visible,
				Config: w.cfg, PenRow: w.penRow, PenCol: w.penCol,
			}
			if w.defined {
				ws.Text = w.Lines()
			}
			ds.Windows = append(ds.Windows, ws)
		}
		snap.Services = append(snap.Services, ds)
	}
	return snap
}

// RestoreManager rebuilds a Manager from a Snapshot.
func RestoreManager(snap ManagerSnapshot) *Manager {
	cols, rows := snap.Cols, snap.Rows
	if cols <= 0 {
		cols = DefaultCols
	}
	if rows <= 0 {
		rows = DefaultRows
	}
	m := NewManager(WithCanvas(cols, rows))
	m.selected = snap.Selected
	m.hasSel = snap.HasSelection
	m.streamSeq = snap.StreamSeq

	for _, ds := range snap.Services {
		d := NewDecoder(ds.Service)
		d.current = ds.Current
		d.DelayTenths = ds.DelayTenths
		for _, ws := range ds.Windows {
			w := &d.windows[ws.ID]
			if !ws.Defined {
				continue
			}
			cfg := ws.Config.withDefaults()
			w.define(cfg)
			w.visible = ws.Visible
			for r, line := range ws.Text {
				if r >= w.rows {
					break
				}
				runes := []rune(line)
				for c := 0; c < w.cols && c < len(runes); c++ {
					w.text[r][c] = runes[c]
				}
			}
			w.penRow, w.penCol = ws.PenRow, ws.PenCol
		}
		m.services[ds.Service] = d
	}

	if m.hasSel {
		if d := m.services[m.selected]; d != nil {
			f := d.Frame(0, m.streamSeq, FrameSwitch, m.cols, m.rows)
			d.latchFrame(f)
		}
	}
	return m
}

// SameState reports whether two managers have equivalent restorable state.
// It compares normalized snapshots, including every independent service.
func SameState(a, b *Manager) bool {
	if a == nil || b == nil {
		return a == b
	}
	return snapshotsEqual(a.Snapshot(), b.Snapshot())
}

func snapshotsEqual(a, b ManagerSnapshot) bool {
	if a.Cols != b.Cols || a.Rows != b.Rows ||
		a.Selected != b.Selected || a.HasSelection != b.HasSelection ||
		a.StreamSeq != b.StreamSeq || len(a.Services) != len(b.Services) {
		return false
	}
	for i := range a.Services {
		sa, sb := a.Services[i], b.Services[i]
		if sa.Service != sb.Service || sa.Current != sb.Current ||
			sa.DelayTenths != sb.DelayTenths || len(sa.Windows) != len(sb.Windows) {
			return false
		}
		for j := range sa.Windows {
			wa, wb := sa.Windows[j], sb.Windows[j]
			if wa.ID != wb.ID || wa.Defined != wb.Defined || wa.Visible != wb.Visible {
				return false
			}
			if !equalStringSlices(wa.Text, wb.Text) {
				return false
			}
		}
	}
	return true
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
