package cea608

import "strings"

func (d *Decoder) appendPacket(packet Packet) {
	if d.Options.RecordPackets {
		d.Packets = append(d.Packets, packet)
	}
}

func (d *Decoder) recordBadControl(p Pair, reason string, parityBad bool) *BadControl {
	d.Diagnostics.BadControls++
	bad := &BadControl{
		Code:        p.Code(),
		ControlCode: p.Code(),
		B1:          p.B1,
		B2:          p.B2,
		FirstByte:   p.B1,
		SecondByte:  p.B2,
		Offset:      p.Offset,
		Index:       p.Index,
		PairIndex:   p.Index,
		Reason:      reason,
		ParityError: parityBad,
	}
	if d.Diagnostics.FirstBadControl == nil {
		d.Diagnostics.FirstBadControl = bad
		d.Diagnostics.FirstBadCode = bad.Code
		d.Diagnostics.FirstBadControlCode = bad.Code
	}
	return bad
}

func isControlLead(b1 byte) bool { return b1 >= 0x10 && b1 <= 0x1f }

func (d *Decoder) rejectBadParity(p Pair) {
	d.Diagnostics.BadPairs++
	d.Diagnostics.ParityErrors++
	d.lastCode = 0
	d.lastKind = ""
	reason := "odd parity check failed"
	if isControlLead(p.B1) {
		d.recordBadControl(p, reason, true)
	}
	d.appendPacket(Packet{Kind: KindBadParity, Pair: p, Reason: reason})
}

func sameCommand(d *Decoder, p Pair, kind PacketKind) bool {
	return d.lastKind == kind && d.lastCode == p.Code()
}

func rememberCommand(d *Decoder, p Pair, kind PacketKind) {
	d.lastCode = p.Code()
	d.lastKind = kind
}

func (d *Decoder) handlePair(p Pair) pairAction {
	if !d.Options.DisableAutoRecover && d.XDS.Active && isControlLead(p.B1) {
		// A 0x10..0x1F control pair explicitly returns the stream to
		// caption/text service. If XDS end/checksum were lost, recover here.
		d.XDS.Abort()
	}
	if consumed, _ := d.XDS.handlePair(p); consumed {
		d.appendPacket(Packet{Kind: KindXDS, Pair: p})
		rememberCommand(d, p, KindXDS)
		return pairConsumed
	}

	if p.B1 == 0 && p.B2 == 0 {
		d.appendPacket(Packet{Kind: KindNull, Pair: p})
		d.lastCode, d.lastKind = 0, KindNull
		return pairConsumed
	}

	if isMiscControl(p.B1, p.B2) {
		name := controlName(p.B1, p.B2)
		if !sameCommand(d, p, KindControl) {
			d.handleControl(p.B2)
		}
		d.appendPacket(Packet{Kind: KindControl, Pair: p, Name: name})
		rememberCommand(d, p, KindControl)
		return pairConsumed
	}

	if row, indent, ok := pacRowIndent(normalizeControlFirst(p.B1), p.B2); ok && isControlLead(p.B1) {
		if !sameCommand(d, p, KindPAC) {
			d.state.pac(d, row, indent)
		}
		d.appendPacket(Packet{Kind: KindPAC, Pair: p})
		rememberCommand(d, p, KindPAC)
		return pairConsumed
	}

	c1 := normalizeControlFirst(p.B1)
	switch {
	case c1 == 0x11 && p.B2 >= 0x30 && p.B2 <= 0x3f:
		if !sameCommand(d, p, KindSpecial) {
			d.emitRune(decodeSpecial(p.B2))
		}
		d.appendPacket(Packet{Kind: KindSpecial, Pair: p})
		rememberCommand(d, p, KindSpecial)
		return pairConsumed

	case (c1 == 0x12 || c1 == 0x13) && p.B2 >= 0x30 && p.B2 <= 0x3f:
		if !sameCommand(d, p, KindExtended) {
			if d.Caption.Col > 0 {
				d.Caption.Col--
			}
			d.emitRune(decodeExtended(c1, p.B2))
		}
		d.appendPacket(Packet{Kind: KindExtended, Pair: p})
		rememberCommand(d, p, KindExtended)
		return pairConsumed

	case c1 == 0x11 && p.B2 >= 0x20 && p.B2 <= 0x2f:
		if !sameCommand(d, p, KindStyle) {
			d.emitRune(' ')
		}
		d.appendPacket(Packet{Kind: KindStyle, Pair: p})
		rememberCommand(d, p, KindStyle)
		return pairConsumed

	case c1 == 0x17 && p.B2 >= 0x21 && p.B2 <= 0x23:
		if !sameCommand(d, p, KindTab) {
			d.Caption.Col += int(p.B2 - 0x20)
		}
		d.appendPacket(Packet{Kind: KindTab, Pair: p})
		rememberCommand(d, p, KindTab)
		return pairConsumed
	}

	if p.B1 >= 0x20 {
		d.emitRune(rune(p.B1))
		if p.B2 >= 0x20 {
			d.emitRune(rune(p.B2))
		}
		d.appendPacket(Packet{Kind: KindCaption, Pair: p})
		rememberCommand(d, p, KindCaption)
		return pairConsumed
	}

	if isControlLead(p.B1) {
		bad := d.recordBadControl(p, "unrecognized or malformed control pair", false)
		d.appendPacket(Packet{Kind: KindBadControl, Pair: p, Reason: bad.Reason})
		// Default pair framing drops the complete bad pair. The opt-in
		// rewind strategy treats B2 as the first byte of the following pair.
		if d.Options.RewindOnBadLead {
			bad.Recovered = true
			return recoverConsumedOne
		}
		return pairConsumed
	}

	// Low non-XDS/non-caption bytes are transport padding in some captures.
	d.appendPacket(Packet{Kind: KindCaption, Pair: p, Reason: "ignored low byte"})
	return pairConsumed
}

func (d *Decoder) emitRune(ch rune) {
	if ch == 0 {
		return
	}
	d.state.putChar(d, ch)
	d.Caption.Text += string(ch)
}

func (d *Decoder) enterMode(m Mode) {
	switch m {
	case ModePopOn:
		d.state = popOnState{}
	case ModePaintOn:
		d.state = paintOnState{}
	case ModeRollUp:
		d.state = rollUpState{}
	case ModeText:
		d.state = textState{}
	default:
		d.state = noneState{}
	}
	d.Caption.Mode = m
	d.Caption.Row = 0
	d.Caption.Col = 0
}

func (d *Decoder) enterRollUp(rows int) {
	d.Caption.RollRows = rows
	d.Caption.BaseRow = Rows - 1
	d.enterMode(ModeRollUp)
	d.Caption.BaseRow = Rows - 1
	d.Caption.Row = d.Caption.BaseRow
}

func (d *Decoder) handleControl(cmd byte) {
	switch cmd {
	case CmdRCL:
		d.enterMode(ModePopOn)
	case CmdRDC:
		d.enterMode(ModePaintOn)
	case CmdRU2:
		d.enterRollUp(2)
	case CmdRU3:
		d.enterRollUp(3)
	case CmdRU4:
		d.enterRollUp(4)
	case CmdRTD, CmdTR:
		d.enterMode(ModeText)
	case CmdEDM:
		d.displayed.clear()
	case CmdENM:
		d.stored.clear()
	case CmdCR:
		d.state.carriageReturn(d)
		d.Caption.Text += "\n"
	case CmdBS:
		d.backspace()
	case CmdDER:
		target := &d.displayed
		if d.Mode() == ModePopOn {
			target = &d.stored
		}
		target.deleteToEnd(d.Caption.Row, d.Caption.Col)
	case CmdEOC:
		d.state.endOfCaption(d)
	default:
		// AOF/AON/FON and other spacing attributes have no text-grid effect.
	}
}

func (d *Decoder) backspace() {
	if d.Caption.Col <= 0 {
		return
	}
	d.Caption.Col--
	target := &d.displayed
	if d.Mode() == ModePopOn {
		target = &d.stored
	}
	target.put(d.Caption.Row, d.Caption.Col, 0)
	runes := []rune(d.Caption.Text)
	if len(runes) > 0 {
		d.Caption.Text = string(runes[:len(runes)-1])
	}
}

func (d *Decoder) appendCueFromDisplay() {
	rows := nonEmptyLines(d.displayed.lines())
	if len(rows) == 0 {
		return
	}
	d.Caption.Cues = append(d.Caption.Cues, Cue{
		Text:  strings.Join(rows, "\n"),
		Lines: rows,
	})
}
