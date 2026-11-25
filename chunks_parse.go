package cafmarker

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
)

type packetEntry struct {
	Size   int64
	Frames int64
}

func parseFormat(ci ChunkInfo, payload []byte) (*Format, error) {
	if len(payload) < 32 {
		return nil, fmt.Errorf("%w: desc payload is %d bytes, need 32", ErrInvalidChunk, len(payload))
	}
	return &Format{
		ChunkInfo:        ci,
		SampleRate:       math.Float64frombits(binary.BigEndian.Uint64(payload[0:8])),
		FormatID:         FourCC(payload[8:12]),
		FormatFlags:      binary.BigEndian.Uint32(payload[12:16]),
		BytesPerPacket:   binary.BigEndian.Uint32(payload[16:20]),
		FramesPerPacket:  binary.BigEndian.Uint32(payload[20:24]),
		ChannelsPerFrame: binary.BigEndian.Uint32(payload[24:28]),
		BitsPerChannel:   binary.BigEndian.Uint32(payload[28:32]),
	}, nil
}

func parsePacketTable(ci ChunkInfo, payload []byte, format *Format) (*PacketTable, []packetEntry, error) {
	if len(payload) < 24 {
		return nil, nil, fmt.Errorf("%w: pakt payload is %d bytes, need 24", ErrInvalidChunk, len(payload))
	}
	count := int64(binary.BigEndian.Uint64(payload[0:8]))
	validFrames := int64(binary.BigEndian.Uint64(payload[8:16]))
	pt := &PacketTable{
		ChunkInfo:         ci,
		NumberPackets:     count,
		NumberValidFrames: validFrames,
		PrimingFrames:     int32(binary.BigEndian.Uint32(payload[16:20])),
		RemainderFrames:   int32(binary.BigEndian.Uint32(payload[20:24])),
	}
	if count < 0 || validFrames < 0 || pt.PrimingFrames < 0 || pt.RemainderFrames < 0 {
		return pt, nil, fmt.Errorf("%w: negative pakt count, frame count, priming, or remainder", ErrInvalidChunk)
	}
	if format != nil && !format.VariableBytes() && !format.VariableFrames() {
		return pt, nil, nil
	}

	pos := 24
	entries := make([]packetEntry, 0, min64(count, 4096))
	for i := int64(0); i < count; i++ {
		var entry packetEntry
		if format == nil || format.VariableBytes() {
			value, n, err := decodeCAFVarInt(payload, pos)
			if err != nil {
				return pt, entries, fmt.Errorf("packet %d size: %w", i, err)
			}
			entry.Size = value
			pos += n
		} else {
			entry.Size = int64(format.BytesPerPacket)
		}
		if format == nil || format.VariableFrames() {
			value, n, err := decodeCAFVarInt(payload, pos)
			if err != nil {
				return pt, entries, fmt.Errorf("packet %d frame count: %w", i, err)
			}
			entry.Frames = value
			pos += n
		} else {
			entry.Frames = int64(format.FramesPerPacket)
		}
		entries = append(entries, entry)
	}
	return pt, entries, nil
}

func decodeCAFVarInt(buf []byte, pos int) (int64, int, error) {
	v := new(big.Int)
	start := pos
	for {
		if pos >= len(buf) {
			return 0, pos - start, io.ErrUnexpectedEOF
		}
		b := buf[pos]
		pos++
		v.Lsh(v, 7)
		v.Or(v, big.NewInt(int64(b&0x7f)))
		if b&0x80 == 0 {
			break
		}
		if pos-start > 10 {
			return 0, pos - start, fmt.Errorf("%w: variable-length integer too long", ErrInvalidChunk)
		}
	}
	if !v.IsInt64() {
		return 0, pos - start, fmt.Errorf("%w: variable-length integer overflows int64", ErrInvalidChunk)
	}
	return v.Int64(), pos - start, nil
}

func parseMarkers(payload []byte) ([]Marker, uint32, error) {
	if len(payload) < 8 {
		return nil, 0, fmt.Errorf("%w: mark payload is %d bytes, need 8", ErrInvalidChunk, len(payload))
	}
	smpteType := binary.BigEndian.Uint32(payload[0:4])
	count := binary.BigEndian.Uint32(payload[4:8])
	capacity := count
	if capacity > 4096 {
		capacity = 4096
	}
	markers := make([]Marker, 0, capacity)
	pos := 8
	for i := uint32(0); i < count; i++ {
		if pos+28 > len(payload) {
			return markers, smpteType, fmt.Errorf("%w: marker %d needs 28 bytes at offset %d", io.ErrUnexpectedEOF, i, pos)
		}
		frameBits := binary.BigEndian.Uint64(payload[pos+4 : pos+12])
		frameFloat := math.Float64frombits(frameBits)
		smpteBytes := payload[pos+16 : pos+24]
		invalid := true
		for _, b := range smpteBytes {
			if b != 0xff {
				invalid = false
				break
			}
		}
		frame := int64(-1)
		inRange := isNonNegativeFiniteFrame(frameFloat)
		integral := inRange && frameFloat == math.Trunc(frameFloat)
		if integral {
			frame = int64(frameFloat)
		}
		mType := binary.BigEndian.Uint32(payload[pos : pos+4])
		markers = append(markers, Marker{
			IndexInChunk:  int(i),
			Type:          mType,
			TypeName:      MarkerTypeName(mType),
			FramePosition: frameFloat,
			Frame:         frame,
			FrameValid:    integral,
			Fractional:    inRange && !integral,
			MarkerID:      binary.BigEndian.Uint32(payload[pos+12 : pos+16]),
			SMPTETime: SMPTETime{
				Hours:                int8(smpteBytes[0]),
				Minutes:              int8(smpteBytes[1]),
				Seconds:              int8(smpteBytes[2]),
				Frames:               int8(smpteBytes[3]),
				SubFrameSampleOffset: binary.BigEndian.Uint32(smpteBytes[4:8]),
				Invalid:              invalid,
			},
			Channel: binary.BigEndian.Uint32(payload[pos+24 : pos+28]),
		})
		pos += 28
	}
	return markers, smpteType, nil
}

func isNonNegativeFiniteFrame(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 && v < float64(math.MaxInt64)
}

func parseStrings(payload []byte, out map[uint32]string) error {
	if len(payload) < 4 {
		return fmt.Errorf("%w: strg payload too short", ErrInvalidChunk)
	}
	count := binary.BigEndian.Uint32(payload[0:4])
	tableBytes := int64(count)*12 + 4
	if tableBytes > int64(len(payload)) {
		return fmt.Errorf("%w: strg ID table needs %d bytes", io.ErrUnexpectedEOF, tableBytes)
	}
	blobStart := int(tableBytes)
	for i := uint32(0); i < count; i++ {
		off := 4 + int(i)*12
		id := binary.BigEndian.Uint32(payload[off : off+4])
		startOffset := int64(binary.BigEndian.Uint64(payload[off+4 : off+12]))
		start := blobStart + int(startOffset)
		if start < blobStart || start >= len(payload) {
			return fmt.Errorf("%w: string %d offset %d is outside blob", ErrInvalidChunk, id, startOffset)
		}
		end := start
		for end < len(payload) && payload[end] != 0 {
			end++
		}
		if end >= len(payload) {
			return fmt.Errorf("%w: string %d is not NUL-terminated", io.ErrUnexpectedEOF, id)
		}
		out[id] = string(payload[start:end])
	}
	return nil
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

// MarkerTypeName returns the CAF marker type name when known.
func MarkerTypeName(t uint32) string {
	if t == 0 {
		return "generic"
	}
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, t)
	s := FourCC(b).String()
	switch s {
	case "pbeg":
		return "program_start"
	case "pend":
		return "program_end"
	case "tbeg":
		return "track_start"
	case "tend":
		return "track_end"
	case "indx":
		return "index"
	case "rbeg":
		return "region_start"
	case "rend":
		return "region_end"
	case "rsyc":
		return "region_sync_point"
	default:
		return s
	}
}

var _ = errors.Is
