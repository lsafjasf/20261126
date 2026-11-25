package cea708

import (
	"errors"
	"testing"
)

func TestParseCommandLifecycle(t *testing.T) {
	payload := visibleTextPayload("BYTE-HELLO", true)
	d := NewDecoder(1)

	n, err := d.ParseCommands(payload)
	if err != nil {
		t.Fatalf("ParseCommands: %v", err)
	}
	if n != len(payload) {
		t.Fatalf("consumed %d of %d bytes", n, len(payload))
	}
	f := d.Frame(1, 0, FramePresent, DefaultCols, DefaultRows)
	if !f.Contains("BYTE-HELLO") {
		t.Fatalf("parsed stream did not show text:\n%s", renderFrame(f))
	}
}

func TestParseTruncatedCommandKeepsPrefix(t *testing.T) {
	d := NewDecoder(1)
	d.DefineWindow(testWindow(0, 1, 1, true))

	// Text applied, then CLW opcode with its mask operand missing.
	payload := []byte{'A', 'B', OpCLW}
	n, err := d.ParseCommands(payload)
	if err == nil {
		t.Fatal("expected truncation diagnostic")
	}
	var diag Diagnostic
	if !errors.As(err, &diag) || diag.Code != "truncated_command" {
		t.Fatalf("expected truncated_command Diagnostic, got %T %v", err, err)
	}
	if n != 2 {
		t.Fatalf("valid prefix should consume 2 bytes, got %d", n)
	}
	if lines := d.Window(0).Lines(); len(lines) == 0 || lines[0] != "AB" {
		t.Fatalf("prefix text should survive, got %q", lines)
	}
	first, ok := d.FirstDiagnostic()
	if !ok || first.Code != "truncated_command" {
		t.Fatalf("decoder did not record first bad code: %+v ok=%v", first, ok)
	}
}

func TestParseUnknownOpcodeStopsSafely(t *testing.T) {
	d := NewDecoder(1)
	n, err := d.ParseCommands([]byte{0x93, 0x00})
	if err == nil {
		t.Fatal("expected unknown opcode diagnostic")
	}
	var diag Diagnostic
	if !errors.As(err, &diag) || diag.Code != "unknown_opcode" {
		t.Fatalf("expected unknown_opcode, got %v", err)
	}
	if n != 0 {
		t.Fatalf("should stop at unknown opcode offset, got consumed=%d", n)
	}
}

func TestParseArbitraryBytesNeverPanic(t *testing.T) {
	d := NewDecoder(1)
	fuzz := []byte{0xFF, 0xFE, 0x9F, 6, 6, 6, 0x88, 0x90, 0x01, 0x02, 0x91, 0x00, 0x80, 0x8F}
	n, _ := d.ParseCommands(fuzz)
	if n > len(fuzz) {
		t.Fatal("parser consumed beyond input")
	}
}

func TestOpcodeOperandTable(t *testing.T) {
	cases := map[byte]int{
		OpCLW: 1, OpDSW: 1, OpHDW: 1, OpTGW: 1, OpDLW: 1,
		OpSPA: 2, OpSPC: 3, OpSPL: 2, OpSWA: 4, OpRST: 0, OpDLC: 0,
	}
	for op, want := range cases {
		got, known := opcodeOperands(op)
		if !known || got != want {
			t.Errorf("opcode %s (0x%02X): operands=%d known=%v want %d", OpcodeName(op), op, got, known, want)
		}
	}
	for op := byte(OpCW0); op <= OpCW7; op++ {
		if n, known := opcodeOperands(byte(op)); !known || n != 0 {
			t.Errorf("CW%d should be zero-operand", op-OpCW0)
		}
	}
	for op := byte(OpDF0); op <= OpDF7; op++ {
		if n, known := opcodeOperands(byte(op)); !known || n != 6 {
			t.Errorf("DF%d should be six-operand", op-OpDF0)
		}
	}
}
