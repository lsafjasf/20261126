package cea708

import "testing"

func TestRecoveryTokenRejectsGarbage(t *testing.T) {
	if _, err := ParseRecoveryPoint("not-base64!!"); err == nil {
		t.Fatal("expected decode error")
	}
	if _, err := ParseRecoveryPoint("aGVsbG8"); err == nil { // base64 "hello"
		t.Fatal("expected json error")
	}
}

func TestJournalEnforcesContiguity(t *testing.T) {
	j := NewJournal()
	if err := j.Append(Item{Seq: 1}); err != nil {
		t.Fatal(err)
	}
	if err := j.Append(Item{Seq: 3}); err == nil {
		t.Fatal("out-of-order append must fail")
	}
	if last, ok := j.Last(); !ok || last.Seq != 1 {
		t.Fatalf("last=%+v ok=%v", last, ok)
	}
	if got := j.From(1); len(got) != 1 {
		t.Fatalf("From(1)=%d items", len(got))
	}
}

func TestPresentWithoutSelection(t *testing.T) {
	m := NewManager()
	if _, ok := m.Present(); ok {
		t.Fatal("Present with no selection must report false")
	}
	if f := m.SelectService(0); f.Service != NoService {
		t.Fatal("invalid service selection should return zero frame")
	}
}

func TestCCAssemblerAbandonsPartialOnNewStart(t *testing.T) {
	asm := &CCAssembler{}
	// size=63 keeps the first "large" packet incomplete.
	asm.Push([]byte{0xFF, 0x3F, 0xAA})
	// A new start abandons the partial packet and completes a size-1 packet.
	asm.Push([]byte{0xFF, 0x01, 0xBB})
	pkts := asm.Take()
	if asm.Resets != 1 {
		t.Fatalf("resets=%d want 1", asm.Resets)
	}
	if len(pkts) != 1 || len(pkts[0]) != 2 || pkts[0][0] != 0x01 || pkts[0][1] != 0xBB {
		t.Fatalf("unexpected completed packets: %x", pkts)
	}
}

func TestCCAssemblerFlushPartial(t *testing.T) {
	asm := &CCAssembler{}
	asm.Push([]byte{0xFF, 0x3F, 0xAA, 0xFE, 0x01, 0x02})
	asm.Flush()
	pkts := asm.Take()
	if len(pkts) != 1 || len(pkts[0]) != 4 {
		t.Fatalf("flush should emit 4 partial bytes, got %x", pkts)
	}
}

func TestIngestRejectsMalformedPacket(t *testing.T) {
	m := NewManager()
	if err := m.IngestPacket(nil); err == nil {
		t.Fatal("nil packet should error")
	}
}

func TestBackspaceAndWrap(t *testing.T) {
	d := NewDecoder(1)
	d.DefineWindow(WindowConfig{ID: 0, RowCount: 2, ColCount: 3, Visible: true})
	d.WriteText("ABCDE") // wraps after the third column
	lines := d.Window(0).Lines()
	if len(lines) != 2 || lines[0] != "ABC" || lines[1] != "DE" {
		t.Fatalf("wrap result=%q", lines)
	}
	d.Backspace()
	lines = d.Window(0).Lines()
	if lines[1] != "D" {
		t.Fatalf("backspace result=%q", lines)
	}
}
