package cea708

import "testing"

func TestPacketServiceBlockRoundTrip(t *testing.T) {
	original := Packet{
		Sequence: 2,
		Blocks: []ServiceBlock{
			block(1, OpRST),
			block(2, []byte("S2")...),
		},
	}
	raw, err := EncodePacket(original.Sequence, original.Blocks...)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodePacket(raw)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Sequence != original.Sequence {
		t.Fatalf("seq=%d want %d", decoded.Sequence, original.Sequence)
	}
	if len(decoded.Blocks) != 2 {
		t.Fatalf("blocks=%d", len(decoded.Blocks))
	}
	if decoded.Blocks[0].Service != 1 || len(decoded.Blocks[0].Data) != 1 ||
		decoded.Blocks[0].Data[0] != OpRST {
		t.Fatalf("block0 mismatch: %+v", decoded.Blocks[0])
	}
	if decoded.Blocks[1].Service != 2 || string(decoded.Blocks[1].Data) != "S2" {
		t.Fatalf("block1 mismatch: %+v", decoded.Blocks[1])
	}
}

func TestExtendedServiceRoundTrip(t *testing.T) {
	raw, err := EncodePacket(0, block(49, OpRST))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodePacket(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded.Blocks) != 1 || decoded.Blocks[0].Service != 49 {
		t.Fatalf("extended service not decoded: %+v", decoded.Blocks)
	}
}

func TestCCDataReassemblyRoundTrip(t *testing.T) {
	p1 := mustPacket(t, 0, block(1, visibleTextPayload("CC-ONE", false)...))
	p2 := mustPacket(t, 1, block(2, visibleTextPayload("CC-TWO", false)...))

	cc := PacketsToCCData(p1, p2)
	asm := &CCAssembler{}
	asm.Push(cc)
	packets := asm.Take()
	if len(packets) != 2 {
		t.Fatalf("expected 2 reassembled packets, got %d (resets=%d)", len(packets), asm.Resets)
	}
	for i, want := range [][]byte{p1, p2} {
		if string(packets[i]) != string(want) {
			t.Fatalf("packet %d mismatch:\n got %x\nwant %x", i, packets[i], want)
		}
	}
}

func TestManagerIngestCCDataPresentsText(t *testing.T) {
	payload := visibleTextPayload("WIRE-TEXT", true)
	pkt := mustPacket(t, 0, block(1, payload...))
	cc := PacketsToCCData(pkt)

	m := NewManager(WithSelectedService(1))
	if err := m.IngestCCData(7, cc); err != nil {
		t.Fatal(err)
	}
	f, ok := m.LastFrame()
	if !ok || f.Service != 1 || !f.Contains("WIRE-TEXT") {
		t.Fatalf("cc_data did not present wire text: ok=%v frame=\n%s", ok, renderFrame(f))
	}
	if f.StreamSeq != 7 {
		t.Fatalf("frame seq=%d want 7", f.StreamSeq)
	}
}

func TestDecoderToleratesTruncatedPacket(t *testing.T) {
	if _, err := DecodePacket([]byte{0x01, 0xE0}); err == nil {
		t.Fatal("expected truncated packet error")
	}
	m := NewManager()
	if err := m.IngestPacket([]byte{}); err == nil {
		t.Fatal("empty packet should error")
	}
}
