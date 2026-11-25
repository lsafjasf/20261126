package cea708

import "fmt"

// ParseCommands interprets one CEA-708 service-block payload against d.
//
// It applies every complete command encountered. If a trailing command is
// truncated, or an unskippable reserved opcode appears, parsing stops at that
// offset, records a Diagnostic on the decoder and returns it. Earlier commands
// remain applied, which lets recovery replay from a clean boundary without
// losing the valid prefix.
func (d *Decoder) ParseCommands(p []byte) (int, error) {
	i := 0
	for i < len(p) {
		b := p[i]
		switch {
		case b == C0NUL || b == C0ETX:
			i++
		case b == C0BS:
			d.Backspace()
			i++
		case b == C0CR:
			d.CarriageReturn()
			i++
		case b == C0HCR:
			d.HorizontalCarriageReturn()
			i++
		case b == C0FF:
			// Form feed net effect: erase the current window. CEA-708's
			// canonical clear is CLW; this keeps raw FF streams sensible.
			if w := d.Window(d.current); w != nil {
				w.clear()
			}
			i++
		case b < 0x20:
			// Other C0 controls are single-byte and intentionally ignored.
			i++
		case b >= 0x20 && b < 0x80:
			d.WriteText(string(mapGraphic(rune(b))))
			i++
		case b >= 0x80 && b < 0xA0:
			n, known := opcodeOperands(b)
			if !known {
				diag := Diagnostic{Offset: i, Opcode: int(b), Code: "unknown_opcode",
					Detail: "reserved C1 opcode; cannot determine operand length"}
				d.addDiag(i, int(b), diag.Code, diag.Detail)
				return i, diag
			}
			if i+1+n > len(p) {
				diag := Diagnostic{Offset: i, Opcode: int(b), Code: "truncated_command",
					Detail: fmt.Sprintf("opcode %s needs %d operand bytes, %d available",
						OpcodeName(b), n, len(p)-i-1)}
				d.addDiag(i, int(b), diag.Code, diag.Detail)
				return i, diag
			}
			args := p[i+1 : i+1+n]
			d.executeCommand(b, args)
			i += 1 + n
		default: // G1 graphic set 0xA0..0xFF maps onto Latin-1/Unicode.
			d.WriteText(string(mapGraphic(rune(b))))
			i++
		}
	}
	return i, nil
}

func (d *Decoder) executeCommand(op byte, args []byte) {
	switch {
	case op >= OpCW0 && op <= OpCW7:
		_ = d.SetCurrentWindow(int(op - OpCW0))
	case op >= OpDF0 && op <= OpDF7:
		d.defineWindowBytes(int(op-OpDF0), args)
	}
	switch op {
	case OpCLW:
		d.ClearWindows(args[0])
	case OpDSW:
		d.DisplayWindows(args[0])
	case OpHDW:
		d.HideWindows(args[0])
	case OpTGW:
		d.ToggleWindows(args[0])
	case OpDLW:
		d.DeleteWindows(args[0])
	case OpDLY:
		d.DelayTenths += int(args[0])
	case OpDLC, OpRST:
		if op == OpRST {
			d.Reset()
		}
	case OpSPA, OpSPC, OpSWA:
		// Pen/window style bytes do not affect the cell model yet; they are
		// consumed here so the stream stays aligned.
	case OpSPL:
		row := int(args[0] & 0x0F)
		col := int(args[1] & 0x3F)
		d.SetPenLocation(row, col)
	}
}

// defineWindowBytes installs geometry from a DFx operand block.
//
// CEA-708 encodes proportional anchor coordinates and a rich style bundle;
// this cell-grid implementation only retains the fields it can render. The
// packing below is the compact representation shared with EncodeDefineWindow;
// callers that need exact bit-level DFx decoding should use WindowConfig.
func (d *Decoder) defineWindowBytes(id int, a []byte) {
	if len(a) < 6 {
		return
	}
	cfg := WindowConfig{
		ID:          id,
		Visible:     a[0]&0x10 != 0,
		AnchorPoint: int(a[0] & 0x0F),
		Priority:    int((a[0] >> 5) & 0x07),
		AnchorRow:   int(a[1]),
		AnchorCol:   int(a[2]),
		RowCount:    int(a[3]&0x0F) + 1,
		ColCount:    int(a[4]&0x3F) + 1,
	}
	d.DefineWindow(cfg)
}

// EncodeDefineWindow returns the six operand bytes for cfg using the compact
// cell packing understood by defineWindowBytes.
func EncodeDefineWindow(cfg WindowConfig) []byte {
	cfg = cfg.withDefaults()
	a := make([]byte, 6)
	if cfg.Visible {
		a[0] |= 0x10
	}
	a[0] |= byte(cfg.AnchorPoint & 0x0F)
	a[0] |= byte(cfg.Priority&0x07) << 5
	a[1] = byte(cfg.AnchorRow)
	a[2] = byte(cfg.AnchorCol)
	a[3] = byte((cfg.RowCount - 1) & 0x0F)
	a[4] = byte((cfg.ColCount - 1) & 0x3F)
	return a
}
