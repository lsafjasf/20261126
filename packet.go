package cea708

import "fmt"

// MaxPacketPayload is the 6-bit packet_size limit of the DTVCC header.
const MaxPacketPayload = 63

// MaxBlockPayload is the 5-bit block_size limit of a service-block header.
const MaxBlockPayload = 31

// ServiceBlock is one service's command payload inside a DTVCC packet.
type ServiceBlock struct {
	Service ServiceID
	Data    []byte
}

// Packet is an assembled DTVCC caption channel packet.
type Packet struct {
	// Sequence is the 2-bit wrapping packet sequence number.
	Sequence uint8
	Blocks   []ServiceBlock
}

// EncodeServiceBlock encodes one block. Services 1..6 use the compact
// three-bit header; service 7..63 use the extended-service escape.
func EncodeServiceBlock(service ServiceID, data []byte) ([]byte, error) {
	if !validService(service) {
		return nil, fmt.Errorf("cea708: invalid service %d", service)
	}
	if len(data) > MaxBlockPayload {
		return nil, fmt.Errorf("cea708: service block payload %d exceeds %d", len(data), MaxBlockPayload)
	}
	n := byte(len(data))
	if service <= 6 {
		out := make([]byte, 1+len(data))
		out[0] = byte(service)<<5 | n
		copy(out[1:], data)
		return out, nil
	}
	out := make([]byte, 2+len(data))
	out[0] = 0xE0 | n // service_number field 111 = extended header
	out[1] = byte(service)
	copy(out[2:], data)
	return out, nil
}

// EncodePacket serializes a DTVCC packet with the given 2-bit sequence.
func EncodePacket(sequence uint8, blocks ...ServiceBlock) ([]byte, error) {
	var body []byte
	for _, b := range blocks {
		enc, err := EncodeServiceBlock(b.Service, b.Data)
		if err != nil {
			return nil, err
		}
		body = append(body, enc...)
	}
	if len(body) > MaxPacketPayload {
		return nil, fmt.Errorf("cea708: packet payload %d exceeds %d; fragment into packets",
			len(body), MaxPacketPayload)
	}
	out := make([]byte, 1+len(body))
	out[0] = (sequence&0x03)<<6 | byte(len(body))&0x3F
	copy(out[1:], body)
	return out, nil
}

// DecodePacket parses a DTVCC packet and its service blocks. It is tolerant of
// padding but reports truncated framing.
func DecodePacket(p []byte) (Packet, error) {
	var pkt Packet
	if len(p) == 0 {
		return pkt, fmt.Errorf("cea708: empty dtvcc packet")
	}
	pkt.Sequence = p[0] >> 6
	size := int(p[0] & 0x3F)
	body := p[1:]
	if size > 0 && size <= len(body) {
		body = body[:size]
	} else if size > len(body) {
		return pkt, fmt.Errorf("cea708: packet_size %d but %d body bytes", size, len(body))
	}

	pos := 0
	for pos < len(body) {
		header := body[pos]
		field := header >> 5
		n := int(header & 0x1F)
		pos++

		var service ServiceID
		if field == 7 {
			if pos >= len(body) {
				return pkt, fmt.Errorf("cea708: truncated extended service header")
			}
			service = ServiceID(body[pos])
			pos++
		} else {
			service = ServiceID(field) // 0 = null/padding block
		}

		if pos+n > len(body) {
			return pkt, fmt.Errorf("cea708: truncated service block for service %d", service)
		}
		data := append([]byte(nil), body[pos:pos+n]...)
		pos += n
		pkt.Blocks = append(pkt.Blocks, ServiceBlock{Service: service, Data: data})
	}
	return pkt, nil
}

// Ingest decodes one wire packet at network sequence seq and routes every
// service block to its independent decoder. The selected service gets one
// presented frame if it had a block in this packet.
func (m *Manager) Ingest(seq uint64, packet []byte) error {
	m.mu.Lock()
	m.streamSeq = seq
	m.mu.Unlock()
	return m.IngestPacket(packet)
}

// IngestPacket decodes and applies a DTVCC packet without changing sequence.
func (m *Manager) IngestPacket(packet []byte) error {
	pkt, err := DecodePacket(packet)
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	touched := make(map[ServiceID]bool)
	var firstCmdErr error
	for _, block := range pkt.Blocks {
		if block.Service == NoService || !validService(block.Service) {
			continue
		}
		d := m.decoderLocked(block.Service)
		if _, cmdErr := d.ParseCommands(block.Data); cmdErr != nil && firstCmdErr == nil {
			firstCmdErr = cmdErr
		}
		touched[block.Service] = true
	}

	for id := range touched {
		d := m.services[id]
		f := d.Frame(0, m.streamSeq, FramePresent, m.cols, m.rows)
		d.latchFrame(f)
		if m.hasSel && m.selected == id {
			m.frameN++
			f.Index = m.frameN
			f.Kind = FramePresent
			d.latchFrame(f)
			m.frames = append(m.frames, f)
		}
	}
	return firstCmdErr
}
