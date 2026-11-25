package aiffmeta

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
)

func parseCOMM(r io.ReaderAt, chunk Chunk, format Format) (*COMM, error) {
	payload, err := readPayload(r, chunk)
	if err != nil {
		return nil, err
	}
	if len(payload) < 18 {
		return nil, fmt.Errorf("payload is %d bytes, COMM needs at least 18", len(payload))
	}
	rate, err := decodeExtended80(payload[8:18])
	comm := &COMM{
		ChunkIndex:      chunk.Index,
		Channels:        binary.BigEndian.Uint16(payload[0:2]),
		SampleFrames:    binary.BigEndian.Uint32(payload[2:6]),
		SampleSize:      binary.BigEndian.Uint16(payload[6:8]),
		SampleRate:      rate,
		ExtendedRateHex: hex.EncodeToString(payload[8:18]),
		IsAIFC:          format == FormatAIFC,
	}
	if err != nil {
		return comm, err
	}
	if !comm.IsAIFC {
		return comm, nil
	}
	if len(payload) < 22 {
		return comm, fmt.Errorf("AIFC COMM payload is %d bytes, need compression type", len(payload))
	}
	comm.CompressionType = string(payload[18:22])
	if len(payload) > 22 {
		name, _, err := parsePString(payload[22:])
		if err != nil {
			return comm, fmt.Errorf("compression name: %w", err)
		}
		comm.CompressionName = name
	}
	return comm, nil
}

func parseSSND(r io.ReaderAt, chunk Chunk) (*SSND, error) {
	if chunk.StoredSize < 8 {
		return nil, fmt.Errorf("payload is %d bytes, SSND needs 8", chunk.StoredSize)
	}
	parameter := make([]byte, 8)
	if _, err := r.ReadAt(parameter, chunk.DataOffset); err != nil {
		return nil, err
	}
	soundOffset := binary.BigEndian.Uint32(parameter[0:4])
	blockSize := binary.BigEndian.Uint32(parameter[4:8])
	payloadEnd := chunk.DataOffset + chunk.StoredSize
	audioStart := chunk.DataOffset + 8 + int64(soundOffset)
	ssnd := &SSND{
		ChunkIndex:  chunk.Index,
		Offset:      soundOffset,
		BlockSize:   blockSize,
		AudioOffset: audioStart,
	}
	if audioStart > payloadEnd {
		return ssnd, fmt.Errorf("sound offset %d exceeds the %d-byte stored payload",
			soundOffset, chunk.StoredSize-8)
	}
	ssnd.AudioSize = payloadEnd - audioStart
	return ssnd, nil
}

func checkSoundLayout(f *File) {
	if f.SSND == nil || f.COMM == nil {
		return
	}
	if f.SSND.AudioSize == 0 && f.COMM.SampleFrames != 0 {
		f.addIssue(f.SSND.AudioOffset, "SSND", "COMM has %d sample frames but no audio bytes follow",
			f.COMM.SampleFrames)
		return
	}
	expected, ok := f.COMM.ExpectedAudioBytes()
	if !ok {
		return
	}
	if expected != f.SSND.AudioSize {
		f.addIssue(f.SSND.AudioOffset, "SSND",
			"COMM implies %d audio byte(s), SSND contains %d; payload is preserved verbatim",
			expected, f.SSND.AudioSize)
	}
}

// decodeExtended80 converts the IEEE 754 80-bit extended value used by AIFF.
// The 64-bit mantissa includes its explicit integer bit.
func decodeExtended80(b []byte) (float64, error) {
	if len(b) != 10 {
		return 0, fmt.Errorf("extended float needs 10 bytes, got %d", len(b))
	}
	sign := b[0] >> 7
	exponent := (uint16(b[0]&0x7f) << 8) | uint16(b[1])
	mantissa := binary.BigEndian.Uint64(b[2:10])

	if exponent == 0 && mantissa == 0 {
		if sign != 0 {
			return math.Copysign(0, -1), nil
		}
		return 0, nil
	}
	if exponent == 0x7fff {
		if mantissa == 0 {
			if sign != 0 {
				return math.Inf(-1), nil
			}
			return math.Inf(1), nil
		}
		return math.NaN(), nil
	}
	if mantissa == 0 {
		return 0, errors.New("invalid extended float with zero mantissa")
	}

	var value float64
	if exponent == 0 {
		value = math.Ldexp(float64(mantissa), 1-16383-63)
	} else {
		value = math.Ldexp(float64(mantissa), int(exponent)-16383-63)
	}
	if sign != 0 {
		value = -value
	}
	return value, nil
}

// parsePString parses an AIFF Pascal string and returns its text and the
// number of bytes consumed, including an odd-byte pad when present.
func parsePString(b []byte) (string, int, error) {
	if len(b) == 0 {
		return "", 0, errors.New("missing length byte")
	}
	n := int(b[0])
	if n > len(b)-1 {
		return "", 0, fmt.Errorf("declares %d bytes, only %d remain", n, len(b)-1)
	}
	consumed := 1 + n
	if consumed&1 != 0 && consumed < len(b) {
		consumed++
	}
	return cleanText(b[1 : 1+n]), consumed, nil
}
