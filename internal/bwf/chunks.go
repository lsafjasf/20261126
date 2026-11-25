package bwf

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
)

func (f *File) ParseFmt() (*Fmt, error) {
	ch := f.Chunk("fmt ")
	if ch == nil {
		return nil, fmt.Errorf("missing fmt chunk")
	}
	b, err := f.Payload(ch, 256)
	if err != nil {
		return nil, fmt.Errorf("read fmt chunk: %w", err)
	}
	if len(b) < 16 {
		return nil, fmt.Errorf("fmt chunk is short: %d bytes", len(b))
	}
	m := &Fmt{
		FormatTag:     binary.LittleEndian.Uint16(b[0:2]),
		Channels:      binary.LittleEndian.Uint16(b[2:4]),
		SampleRate:    binary.LittleEndian.Uint32(b[4:8]),
		ByteRate:      binary.LittleEndian.Uint32(b[8:12]),
		BlockAlign:    binary.LittleEndian.Uint16(b[12:14]),
		BitsPerSample: binary.LittleEndian.Uint16(b[14:16]),
		PayloadSize:   ch.Size,
	}
	if m.FormatTag == 0xFFFE && len(b) >= 40 {
		m.Extensible = true
		m.ValidBits = binary.LittleEndian.Uint16(b[18:20])
		m.ChannelMask = binary.LittleEndian.Uint32(b[20:24])
		m.SubFormatTag = binary.LittleEndian.Uint16(b[24:26])
	}
	baseTag := m.FormatTag
	if m.Extensible {
		baseTag = m.SubFormatTag
	}
	m.Compressed = baseTag != 1 && baseTag != 3
	return m, nil
}

func (f *File) ParseBEXT() (*BEXT, error) {
	ch := f.Chunk("bext")
	if ch == nil {
		return nil, fmt.Errorf("missing bext chunk")
	}
	b, err := f.Payload(ch, 70<<10)
	if err != nil {
		return nil, fmt.Errorf("read bext chunk: %w", err)
	}
	x := &BEXT{PayloadSize: ch.Size}
	if len(b) < 348 {
		x.Short = true
	}
	x.Description = cstring(b, 0, 256)
	x.Originator = cstring(b, 256, 32)
	x.OriginatorRef = cstring(b, 288, 32)
	x.OriginationDate = cstring(b, 320, 10)
	x.OriginationTime = cstring(b, 330, 8)
	if len(b) >= 346 {
		x.TimeReferenceLow = binary.LittleEndian.Uint32(b[338:342])
		x.TimeReferenceHigh = binary.LittleEndian.Uint32(b[342:346])
		x.TimeReference = uint64(x.TimeReferenceLow) | uint64(x.TimeReferenceHigh)<<32
	}
	if len(b) >= 348 {
		x.Version = binary.LittleEndian.Uint16(b[346:348])
	}
	if len(b) >= 412 {
		umid := b[348:412]
		nonzero := false
		for _, c := range umid {
			if c != 0 {
				nonzero = true
				break
			}
		}
		if nonzero {
			x.UMID = hex.EncodeToString(umid)
		}
	}
	return x, nil
}

func cstring(b []byte, off, n int) string {
	if off >= len(b) {
		return ""
	}
	end := off + n
	if end > len(b) {
		end = len(b)
	}
	v := b[off:end]
	if i := strings.IndexByte(string(v), 0); i >= 0 {
		v = v[:i]
	}
	return strings.TrimSpace(string(v))
}
