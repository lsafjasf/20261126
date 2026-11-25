package main

import (
	"time"

	cea708 "cea708"
)

func dfCommand(id int, cfg cea708.WindowConfig) []byte {
	out := []byte{cea708.OpDF0 + byte(id)}
	return append(out, cea708.EncodeDefineWindow(cfg)...)
}

// captionPayload is a DF0 + SPL + text block that displays one line.
func captionPayload(text string) []byte {
	cfg := cea708.WindowConfig{
		ID: 0, RowCount: 2, ColCount: 24,
		AnchorRow: 2, AnchorCol: 2, Visible: true,
	}
	out := dfCommand(0, cfg)
	out = append(out, cea708.OpSPL, 0x00, 0x00)
	out = append(out, []byte(text)...)
	return out
}

func mustPacket(seq uint8, service cea708.ServiceID, payload ...byte) []byte {
	p, err := cea708.EncodePacket(seq, cea708.ServiceBlock{Service: service, Data: payload})
	if err != nil {
		panic(err)
	}
	return p
}

// switchScript is the reproducible "program switches service 1 <-> 2" feed,
// including clean reset boundaries usable as recovery points.
func switchScript() []cea708.Item {
	s1First := append([]byte{cea708.OpRST}, captionPayload("S1: EVENING NEWS")...)
	pkt1 := mustPacket(0, 1, s1First...)

	s1Second := append([]byte{cea708.C0CR}, []byte("S1: WEATHER LATER")...)
	pkt2 := mustPacket(1, 1, s1Second...)

	s2First := append([]byte{cea708.OpRST}, captionPayload("S2: MATCH 1 - 0")...)
	pkt3 := mustPacket(2, 2, s2First...)

	s2Second := append([]byte{cea708.C0CR}, []byte("S2: 78' GOAL!")...)
	pkt4 := mustPacket(3, 2, s2Second...)

	return []cea708.Item{
		{Seq: 1, PTS: 0, Packet: pkt1, Select: 1, Reset: true},
		{Seq: 2, PTS: time.Second, Packet: pkt2},
		{Seq: 3, PTS: 2 * time.Second, Packet: pkt3, Select: 2, Reset: true},
		{Seq: 4, PTS: 3 * time.Second, Packet: pkt4},
		{Seq: 5, PTS: 4 * time.Second, Select: 1},
	}
}

// RuleStep is one labeled clear-rules demonstration frame.
type RuleStep struct {
	Step        int          `json:"step"`
	Rule        string       `json:"rule"`
	Explanation string       `json:"explanation"`
	Frame       cea708.Frame `json:"frame"`
}

// clearRuleSteps exercises CLW / HDW+DSW / DLW on service 1.
func clearRuleSteps() []RuleStep {
	m := cea708.NewManager(cea708.WithSelectedService(1))
	step := 0
	collect := func(rule, explain string) RuleStep {
		step++
		f, _ := m.LastFrame()
		return RuleStep{Step: step, Rule: rule, Explanation: explain, Frame: f}
	}

	m.Update(1, func(d *cea708.Decoder) {
		d.DefineWindow(cea708.WindowConfig{
			ID: 0, RowCount: 1, ColCount: 22, AnchorRow: 2, AnchorCol: 3, Visible: true,
		})
		d.WriteText("CAPTION TEXT")
	})
	steps := []RuleStep{collect("draw", "DF0 defines a visible window and text is drawn")}

	m.Update(1, func(d *cea708.Decoder) { d.ClearWindows(0x01) })
	steps = append(steps, collect("CLW", "ClearWindows erases glyphs; window stays defined/visible"))

	m.Update(1, func(d *cea708.Decoder) { d.WriteText("HIDDEN BUT KEPT") })
	m.Update(1, func(d *cea708.Decoder) { d.HideWindows(0x01) })
	steps = append(steps, collect("HDW", "HideWindows removes output but retains the text buffer"))

	m.Update(1, func(d *cea708.Decoder) { d.DisplayWindows(0x01) })
	steps = append(steps, collect("DSW", "DisplayWindows brings the retained text back"))

	m.Update(1, func(d *cea708.Decoder) { d.DeleteWindows(0x01) })
	steps = append(steps, collect("DLW", "DeleteWindows destroys definition and contents"))

	return steps
}

// RecoveryInfo summarizes the outage/rebuild demonstration.
type RecoveryInfo struct {
	CheckpointSeq cea708.Seq           `json:"checkpoint_seq"`
	Boundary      cea708.Seq           `json:"boundary"`
	Token         string               `json:"token"`
	OutageError   string               `json:"outage_error,omitempty"`
	Rebuild       cea708.RebuildReport `json:"rebuild"`
	StateEqual    bool                 `json:"state_equal"`
}

// buildRecovery applies the prefix through seq 3, checkpoints there, then
// continues the live session while a second manager rebuilds from the token
// plus suffix replay after a simulated outage.
func buildRecovery(items []cea708.Item) ([]cea708.Frame, []cea708.ClearRecord, RecoveryInfo, error) {
	ref := cea708.NewManager()
	rs := cea708.NewRecoverySession(ref)
	for _, it := range items {
		if it.Seq <= 3 {
			if _, err := rs.Ingest(it); err != nil {
				return nil, nil, RecoveryInfo{}, err
			}
		}
	}

	cp := rs.Checkpoint(true)
	token, err := cp.Token()
	if err != nil {
		return nil, nil, RecoveryInfo{}, err
	}
	parsed, err := cea708.ParseRecoveryPoint(token)
	if err != nil {
		return nil, nil, RecoveryInfo{}, err
	}

	source := &cea708.MemSource{Items: items, Down: true}
	info := RecoveryInfo{CheckpointSeq: cp.Seq, Boundary: cp.Boundary, Token: token}
	if _, _, downErr := cea708.Rebuild(parsed, source, 0); downErr != nil {
		info.OutageError = downErr.Error()
	}

	source.Down = false
	rebuilt, rep, err := cea708.Rebuild(parsed, source, 0)
	if err != nil {
		return ref.Frames(), ref.ClearEvents(), info, err
	}
	info.Rebuild = rep

	// Continue the original live session through seq 4-5.
	for _, it := range items {
		if it.Seq > 3 {
			if _, err := rs.Ingest(it); err != nil {
				return ref.Frames(), ref.ClearEvents(), info, err
			}
		}
	}
	info.StateEqual = cea708.SameState(ref, rebuilt)
	return ref.Frames(), ref.ClearEvents(), info, nil
}
