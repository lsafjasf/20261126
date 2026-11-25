package cea708

import "testing"

func dfCommand(id int, cfg WindowConfig) []byte {
	cmd := []byte{OpDF0 + byte(id)}
	return append(cmd, EncodeDefineWindow(cfg)...)
}

func mustPacket(t *testing.T, seq uint8, blocks ...ServiceBlock) []byte {
	t.Helper()
	p, err := EncodePacket(seq, blocks...)
	if err != nil {
		t.Fatalf("EncodePacket: %v", err)
	}
	return p
}

func block(service ServiceID, data ...byte) ServiceBlock {
	return ServiceBlock{Service: service, Data: data}
}

// visibleTextPayload builds a DF0 + SPL + text + optional DSW payload that the
// byte parser can turn into an on-screen line.
func visibleTextPayload(text string, showViaDSW bool) []byte {
	cfg := WindowConfig{
		ID: 0, RowCount: 2, ColCount: 24,
		AnchorRow: 2, AnchorCol: 2, Visible: !showViaDSW,
	}
	p := append([]byte{}, dfCommand(0, cfg)...)
	p = append(p, OpSPL, 0x00, 0x00)
	p = append(p, []byte(text)...)
	if showViaDSW {
		p = append(p, OpDSW, 0x01)
	}
	return p
}
