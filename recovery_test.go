package cea708

import (
	"errors"
	"testing"
	"time"
)

// recoveryScript models a program that starts on service 1, reaches a clean
// reset boundary, switches to service 2 at seq 3, then switches back at seq 5.
func recoveryScript(t *testing.T) []Item {
	t.Helper()

	s1Reset := append([]byte{OpRST}, visibleTextPayload("S1 START", true)...)
	pkt1 := mustPacket(t, 0, block(1, s1Reset...))

	s1Second := append([]byte{C0CR}, []byte("S1 SECOND")...)
	pkt2 := mustPacket(t, 1, block(1, s1Second...))

	s2Reset := append([]byte{OpRST}, visibleTextPayload("S2 HELLO", true)...)
	pkt3 := mustPacket(t, 2, block(2, s2Reset...))

	s2Second := append([]byte{C0CR}, []byte("S2 AGAIN")...)
	pkt4 := mustPacket(t, 3, block(2, s2Second...))

	return []Item{
		{Seq: 1, PTS: 0, Packet: pkt1, Select: 1, Reset: true},
		{Seq: 2, PTS: time.Second, Packet: pkt2},
		{Seq: 3, PTS: 2 * time.Second, Packet: pkt3, Select: 2, Reset: true},
		{Seq: 4, PTS: 3 * time.Second, Packet: pkt4},
		{Seq: 5, PTS: 4 * time.Second, Select: 1},
	}
}

func TestRebuildFromBoundaryMatchesUninterrupted(t *testing.T) {
	items := recoveryScript(t)
	ref, refRS, err := ReplayAll(items)
	if err != nil {
		t.Fatal(err)
	}
	if refRS.Applied != 5 || refRS.Boundary != 3 {
		t.Fatalf("session state applied=%d boundary=%d", refRS.Applied, refRS.Boundary)
	}

	last, _ := ref.LastFrame()
	if last.Service != 1 || !last.Contains("S1 START") || !last.Contains("S1 SECOND") {
		t.Fatalf("reference final screen wrong:\n%s", renderFrame(last))
	}
	if last.Contains("S2 ") {
		t.Fatalf("service 2 ghost on final service-1 screen:\n%s", renderFrame(last))
	}

	rp := RecoveryPoint{Seq: refRS.Applied, Boundary: 1}
	built, rep, err := Rebuild(rp, &MemSource{Items: items}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if rep.UsedSnapshot || rep.Applied != 5 || rep.FinalSeq != 5 {
		t.Fatalf("unexpected rebuild report: %+v", rep)
	}
	if !SameState(ref, built) {
		t.Fatalf("boundary rebuild state differs from uninterrupted decode\nref=%+v\nbuilt=%+v",
			ref.Snapshot(), built.Snapshot())
	}
}

func TestRebuildFromSnapshotTokenAndSuffix(t *testing.T) {
	items := recoveryScript(t)
	ref, _, err := ReplayAll(items)
	if err != nil {
		t.Fatal(err)
	}

	prefix, rs, err := ReplayAll(items[:3])
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := prefix.Selected(); got != 2 {
		t.Fatalf("prefix should be on service 2, got %d", got)
	}

	cp := rs.Checkpoint(true)
	if cp.Snapshot == nil || cp.Seq != 3 || cp.Boundary != 3 {
		t.Fatalf("bad checkpoint: %+v", cp)
	}
	token, err := cp.Token()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseRecoveryPoint(token)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Snapshot == nil || parsed.Seq != 3 {
		t.Fatal("token lost snapshot/seq")
	}

	built, rep, err := Rebuild(parsed, &MemSource{Items: items}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.UsedSnapshot || rep.Applied != 2 {
		t.Fatalf("snapshot rebuild should replay seq 4-5, report=%+v", rep)
	}
	if !SameState(ref, built) {
		t.Fatalf("snapshot rebuild differs\nref=%+v\nbuilt=%+v", ref.Snapshot(), built.Snapshot())
	}
}

func TestReplayedOverlapIsDeduplicated(t *testing.T) {
	items := recoveryScript(t)
	m, rs, err := ReplayAll(items)
	if err != nil {
		t.Fatal(err)
	}
	framesBefore := len(m.Frames())

	rep, err := rs.Ingest(items[2]) // redeliver seq 3
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Duplicate || rep.Applied {
		t.Fatalf("redelivery must be duplicate-skipped: %+v", rep)
	}
	if len(m.Frames()) != framesBefore {
		t.Fatal("duplicate item must not present another frame")
	}
	if rs.Applied != 5 {
		t.Fatalf("applied high-water moved: %d", rs.Applied)
	}
}

func TestGapRejected(t *testing.T) {
	rs := NewRecoverySession(NewManager())
	_, err := rs.Ingest(recoveryScript(t)[1]) // seq 2 without seq 1
	if !errors.Is(err, ErrSequenceGap) {
		t.Fatalf("expected ErrSequenceGap, got %v", err)
	}
}

func TestNetworkOutageThenResume(t *testing.T) {
	items := recoveryScript(t)
	src := &MemSource{Items: items, Down: true}

	_, _, err := Rebuild(RecoveryPoint{Boundary: 1}, src, 0)
	if !errors.Is(err, ErrNetworkDown) {
		t.Fatalf("expected network error, got %v", err)
	}

	src.Down = false
	m, rep, err := Rebuild(RecoveryPoint{Boundary: 1}, src, 0)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Applied != 5 {
		t.Fatalf("resume should rebuild all 5 items, got %+v", rep)
	}
	if f, ok := m.LastFrame(); !ok || f.Service != 1 || !f.Contains("S1 START") {
		t.Fatalf("resumed rebuild wrong screen: ok=%v\n%s", ok, renderFrame(f))
	}
}

func TestRebuildToTargetStopsAtPoint(t *testing.T) {
	items := recoveryScript(t)
	m, rep, err := Rebuild(RecoveryPoint{Boundary: 1}, &MemSource{Items: items}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if rep.FinalSeq != 3 {
		t.Fatalf("target rebuild final=%d want 3", rep.FinalSeq)
	}
	if id, ok := m.Selected(); !ok || id != 2 {
		t.Fatalf("target 3 should rest on service 2, id=%d ok=%v", id, ok)
	}
}
