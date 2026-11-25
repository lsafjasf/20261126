package cea608

// BeginXDS starts an explicitly supplied XDS header. Most callers should let
// Decode detect XDS pairs; this entry point is useful for extracted pair
// streams that already know the packet boundary.
func (d *Decoder) BeginXDS(class, typ byte) {
	d.XDS.beginHeader(class, typ, 0)
}

func (x *XDSState) beginHeader(b1, b2 byte, offset int) {
	if x.Active {
		x.Truncated++
	}
	x.Active = true
	x.HasHeader = true
	x.ClassCode = b1
	x.TypeCode = b2
	x.Class = int(b1 & 0x0f)
	x.Type = int(b2 & 0x0f)
	x.Content = x.Content[:0]
	x.raw = append(x.raw[:0], b1, b2)
	x.StartOffset = offset
	x.EndOffset = 0
}

func isXDSFirstByte(b1 byte) bool {
	return b1 >= 0x01 && b1 <= 0x0f
}

func isXDSEndPair(p Pair) bool {
	// The wire byte is commonly 0x8F (odd parity); masking bit 7 leaves 0x0F.
	return p.B1 == 0x0f || p.Raw1 == 0x8f
}

// handlePair consumes one pair while an XDS packet is active. It reports true
// when the pair belonged to XDS state.
func (x *XDSState) handlePair(p Pair) (done bool, packet *XDSPacket) {
	if !x.Active {
		if isXDSFirstByte(p.B1) {
			x.beginHeader(p.B1, p.B2, p.Offset)
			return true, nil
		}
		return false, nil
	}

	if isXDSEndPair(p) {
		x.raw = append(x.raw, p.B1, p.B2)
		packet = x.finish(p)
		return true, packet
	}

	if !x.HasHeader {
		x.beginHeader(p.B1, p.B2, p.Offset)
		return true, nil
	}

	if len(x.Content) < 32 {
		remaining := 32 - len(x.Content)
		if remaining >= 1 {
			x.Content = append(x.Content, p.B1)
		}
		if remaining >= 2 {
			x.Content = append(x.Content, p.B2)
		}
	}
	x.raw = append(x.raw, p.B1, p.B2)
	return true, nil
}

func (x *XDSState) finish(end Pair) *XDSPacket {
	content := append([]byte(nil), x.Content...)
	raw := append([]byte(nil), x.raw...)
	checksum := end.B2 & 0x7f
	sum := 0
	for _, b := range raw {
		sum += int(b & 0x7f)
	}
	ok := sum%0x80 == 0
	if !ok {
		x.BadChecksums++
	}
	if len(content) > x.MaxPacketSize {
		x.MaxPacketSize = len(content)
	}
	packet := &XDSPacket{
		Class:       x.Class,
		Type:        x.Type,
		ClassCode:   x.ClassCode,
		TypeCode:    x.TypeCode,
		Content:     content,
		Raw:         raw,
		Checksum:    checksum,
		ChecksumOK:  ok,
		StartOffset: x.StartOffset,
		EndOffset:   end.Offset + 2,
	}
	x.Packets = append(x.Packets, *packet)
	x.Active = false
	x.HasHeader = false
	x.Class = 0
	x.Type = 0
	x.ClassCode = 0
	x.TypeCode = 0
	x.Content = nil
	x.raw = nil
	return packet
}

// Abort drops a partially received XDS packet without touching caption state.
func (x *XDSState) Abort() {
	if x.Active {
		x.Truncated++
	}
	x.Active = false
	x.HasHeader = false
	x.Content = nil
	x.raw = nil
}

// Completed returns a copy of completed XDS packets.
func (x *XDSState) Completed() []XDSPacket {
	return append([]XDSPacket(nil), x.Packets...)
}
