package cea708

// CEA-708 C1 window/pen command opcodes. CW0..CW7 and DF0..DF7 encode the
// window index in the low three bits.
const (
	OpCW0 = 0x80 // SetCurrentWindow 0 (through OpCW7 = 0x87)
	OpCW7 = 0x87
	OpCLW = 0x88 // ClearWindows, 1 mask byte
	OpDSW = 0x89 // DisplayWindows, 1 mask byte
	OpHDW = 0x8A // HideWindows, 1 mask byte
	OpTGW = 0x8B // ToggleWindows, 1 mask byte
	OpDLW = 0x8C // DeleteWindows, 1 mask byte
	OpDLY = 0x8D // Delay, 1 operand (tenths of a second)
	OpDLC = 0x8E // DelayCancel
	OpRST = 0x8F // Reset
	OpSPA = 0x90 // SetPenAttributes, 2 operands
	OpSPC = 0x91 // SetPenColor, 3 operands
	OpSPL = 0x92 // SetPenLocation, 2 operands
	OpSWA = 0x97 // SetWindowAttributes, 4 operands
	OpDF0 = 0x98 // DefineWindow 0 (through OpDF7 = 0x9F), 6 operands
	OpDF7 = 0x9F
)

// C0 control codes handled by the interpreter.
const (
	C0NUL = 0x00
	C0ETX = 0x03
	C0BS  = 0x08
	C0FF  = 0x0C
	C0CR  = 0x0D
	C0HCR = 0x0E
)

// opcodeOperands reports how many operand bytes follow a C1 command opcode.
// unknown is true for reserved/unrecognized opcodes whose skip length cannot
// be determined safely.
func opcodeOperands(op byte) (n int, known bool) {
	switch {
	case op >= OpCW0 && op <= OpCW7:
		return 0, true
	case op >= OpDF0 && op <= OpDF7:
		return 6, true
	}
	switch op {
	case OpDLC, OpRST:
		return 0, true
	case OpCLW, OpDSW, OpHDW, OpTGW, OpDLW, OpDLY:
		return 1, true
	case OpSPA, OpSPL:
		return 2, true
	case OpSPC:
		return 3, true
	case OpSWA:
		return 4, true
	default:
		return 0, false
	}
}

// OpcodeName returns the canonical mnemonic for a byte (best effort).
func OpcodeName(op byte) string {
	switch {
	case op >= OpCW0 && op <= OpCW7:
		return "CW" + string(rune('0'+op-OpCW0))
	case op >= OpDF0 && op <= OpDF7:
		return "DF" + string(rune('0'+op-OpDF0))
	}
	switch op {
	case OpCLW:
		return "CLW"
	case OpDSW:
		return "DSW"
	case OpHDW:
		return "HDW"
	case OpTGW:
		return "TGW"
	case OpDLW:
		return "DLW"
	case OpDLY:
		return "DLY"
	case OpDLC:
		return "DLC"
	case OpRST:
		return "RST"
	case OpSPA:
		return "SPA"
	case OpSPC:
		return "SPC"
	case OpSPL:
		return "SPL"
	case OpSWA:
		return "SWA"
	default:
		if op < 0x20 {
			return "C0"
		}
		if op >= 0x80 && op < 0xA0 {
			return "C1?"
		}
		return "GLYPH"
	}
}
