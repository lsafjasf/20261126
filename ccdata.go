package cea708

// cc_data triple header masks (CEA-708 / A/53 cc_data()).
const (
	ccMarkerMask = 0xF8 // top five bits are always 11111
	ccValidBit   = 0x04
	ccTypeMask   = 0x03

	ccType608F1    = 0
	ccType608F2    = 1
	ccTypeDTVCont  = 2 // DTVCC packet continuation pair
	ccTypeDTVStart = 3 // DTVCC packet start pair
)

// CCTriple builds one valid cc_data 3-byte unit. ccType is 0..3.
func CCTriple(ccType byte, d1, d2 byte) [3]byte {
	return [3]byte{0xF8 | ccValidBit | (ccType & ccTypeMask), d1, d2}
}

// CCAssembler reassembles DTVCC packets from cc_data triples. It ignores
// CEA-608 triples and is resilient to lost starts or trailing padding.
type CCAssembler struct {
	pending   []byte
	active    bool
	Completed [][]byte // packets completed since the last Take call
	Resets    int      // starts that had to abandon a partial packet
}

// Take returns and clears newly completed packets.
func (a *CCAssembler) Take() [][]byte {
	out := a.Completed
	a.Completed = nil
	return out
}

// Flush emits any partially assembled packet (e.g. at a segment boundary).
func (a *CCAssembler) Flush() {
	if a.active && len(a.pending) > 0 {
		pkt := append([]byte(nil), a.pending...)
		a.Completed = append(a.Completed, pkt)
	}
	a.pending = nil
	a.active = false
}

// Push consumes cc_data bytes in groups of three. Malformed trailing bytes
// (< 3) are ignored. Completed packets are appended to Completed.
func (a *CCAssembler) Push(ccData []byte) {
	for i := 0; i+2 < len(ccData); i += 3 {
		h := ccData[i]
		if h&ccMarkerMask != ccMarkerMask {
			continue
		}
		if h&ccValidBit == 0 {
			continue
		}
		switch h & ccTypeMask {
		case ccType608F1, ccType608F2:
			continue // CEA-608 / XDS is a different service plane
		case ccTypeDTVStart:
			if a.active && len(a.pending) > 1 {
				// New start abandons an unterminated packet: count a reset so
				// callers can align recovery to the next clean boundary.
				a.Resets++
			}
			a.active = true
			a.pending = []byte{ccData[i+1], ccData[i+2]}
			a.checkComplete()
		case ccTypeDTVCont:
			if !a.active {
				continue
			}
			a.pending = append(a.pending, ccData[i+1], ccData[i+2])
			a.checkComplete()
		}
	}
}

func (a *CCAssembler) checkComplete() {
	if len(a.pending) == 0 {
		return
	}
	want := 1 + int(a.pending[0]&0x3F) // header + packet_size body bytes
	if want <= 1 || len(a.pending) < want {
		return
	}
	pkt := append([]byte(nil), a.pending[:want]...)
	a.Completed = append(a.Completed, pkt)
	a.pending = nil
	a.active = false
}

// PacketsToCCData serializes assembled DTVCC packets back into valid cc_data
// triples using a start triple followed by continuation triples. It is mainly
// useful for fixtures and round-trip tests.
func PacketsToCCData(packets ...[]byte) []byte {
	var out []byte
	for _, pkt := range packets {
		if len(pkt) == 0 {
			continue
		}
		first := pkt[0]
		second := byte(0)
		if len(pkt) > 1 {
			second = pkt[1]
		}
		t := CCTriple(ccTypeDTVStart, first, second)
		out = append(out, t[0], t[1], t[2])
		for i := 2; i < len(pkt); i += 2 {
			d1 := pkt[i]
			d2 := byte(0)
			if i+1 < len(pkt) {
				d2 = pkt[i+1]
			}
			t = CCTriple(ccTypeDTVCont, d1, d2)
			out = append(out, t[0], t[1], t[2])
		}
	}
	return out
}

// IngestCCData reassembles packets from cc_data triples and applies them.
func (m *Manager) IngestCCData(seq uint64, ccData []byte) error {
	asm := &CCAssembler{}
	asm.Push(ccData)
	for _, pkt := range asm.Take() {
		if err := m.Ingest(seq, pkt); err != nil {
			return err
		}
	}
	return nil
}
