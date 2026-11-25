package wavfix

import (
	"encoding/binary"
	"fmt"
	"math"
)

// Parse reads a RIFF/WAVE byte slice and builds its chunk offset table.
// It tolerates missing fact and incorrect metadata lengths; structural
// truncation or missing fmt/data is returned as an error.
func Parse(raw []byte) (*File, error) {
	if len(raw) < 12 || string(raw[0:4]) != "RIFF" || string(raw[8:12]) != "WAVE" {
		return nil, ErrNotRIFF
	}

	file := &File{
		Raw:      raw,
		RIFFSize: binary.LittleEndian.Uint32(raw[4:8]),
	}
	actualRIFFSize := len(raw) - 8
	if uint64(file.RIFFSize) != uint64(actualRIFFSize) {
		file.Reports = append(file.Reports, Issue{
			Code:    "riff_size_mismatch",
			Chunk:   "RIFF",
			Offset:  4,
			Message: fmt.Sprintf("RIFF size field is %d, actual RIFF payload size is %d", file.RIFFSize, actualRIFFSize),
		})
	}

	offset := 12
	for offset < len(raw) {
		if len(raw)-offset < 8 {
			file.Trailing = append(file.Trailing, raw[offset:]...)
			file.Reports = append(file.Reports, Issue{
				Code:    "short_chunk_header",
				Offset:  offset,
				Message: fmt.Sprintf("%d trailing byte(s) cannot form a chunk header", len(raw)-offset),
			})
			break
		}

		chunk := &Chunk{
			Index:        len(file.Chunks),
			ID:           string(raw[offset : offset+4]),
			Offset:       offset,
			DeclaredSize: binary.LittleEndian.Uint32(raw[offset+4 : offset+8]),
			DataOffset:   offset + 8,
		}

		size64 := uint64(chunk.DeclaredSize)
		end64 := uint64(chunk.DataOffset) + size64
		if size64 > math.MaxInt || end64 > uint64(len(raw)) {
			chunk.Truncated = true
			chunk.Payload = raw[chunk.DataOffset:]
			chunk.Raw = raw[chunk.Offset:]
			chunk.NextOffset = len(raw)
			file.Chunks = append(file.Chunks, chunk)
			if chunk.ID == "data" && file.Data == nil {
				file.Data = &DataChunk{
					ChunkIndex: chunk.Index,
					Offset:     chunk.DataOffset,
					Size:       chunk.DeclaredSize,
					Payload:    chunk.Payload,
				}
			}
			if chunk.ID == "fact" && file.Fact == nil {
				file.Fact = parseFact(chunk)
			}
			file.Reports = append(file.Reports, Issue{
				Code:    "truncated_chunk",
				Chunk:   chunk.ID,
				Offset:  chunk.Offset,
				Message: fmt.Sprintf("chunk declares %d payload bytes but only %d remain", chunk.DeclaredSize, len(chunk.Payload)),
			})
			return file, fmt.Errorf("%w: %s at offset %d", ErrTruncated, chunk.ID, chunk.Offset)
		}

		end := int(end64)
		chunk.Payload = raw[chunk.DataOffset:end]
		chunk.Padding = int(chunk.DeclaredSize & 1)
		chunk.NextOffset = end + chunk.Padding
		if chunk.NextOffset > len(raw) {
			chunk.Padding = 0
			chunk.NextOffset = end
			file.Reports = append(file.Reports, Issue{
				Code:    "missing_pad_byte",
				Chunk:   chunk.ID,
				Offset:  end,
				Message: "odd-length chunk has no RIFF alignment pad byte",
			})
		}
		chunk.Raw = raw[chunk.Offset:chunk.NextOffset]
		file.Chunks = append(file.Chunks, chunk)

		switch chunk.ID {
		case "fmt ":
			format, err := parseFormat(chunk)
			if err != nil {
				return file, err
			}
			if file.Format == nil {
				file.Format = format
			} else {
				file.Reports = append(file.Reports, Issue{Code: "duplicate_fmt", Chunk: chunk.ID, Offset: chunk.Offset, Message: "multiple fmt chunks; using the first"})
			}
		case "fact":
			fact := parseFact(chunk)
			if file.Fact == nil {
				file.Fact = fact
			} else {
				file.Reports = append(file.Reports, Issue{Code: "duplicate_fact", Chunk: chunk.ID, Offset: chunk.Offset, Message: "multiple fact chunks; repairing the first and replacing the set"})
			}
		case "data":
			data := &DataChunk{
				ChunkIndex: chunk.Index,
				Offset:     chunk.DataOffset,
				Size:       chunk.DeclaredSize,
				Payload:    chunk.Payload,
			}
			if file.Data == nil {
				file.Data = data
			} else {
				file.Reports = append(file.Reports, Issue{Code: "duplicate_data", Chunk: chunk.ID, Offset: chunk.Offset, Message: "multiple data chunks; using the first for timing"})
			}
		}

		offset = chunk.NextOffset
	}

	if file.Format == nil {
		return file, ErrNoFormat
	}
	if file.Data == nil {
		return file, ErrNoData
	}
	return file, nil
}

func parseFormat(chunk *Chunk) (*FormatChunk, error) {
	payload := chunk.Payload
	if len(payload) < 16 {
		return nil, fmt.Errorf("%w at offset %d: need 16 bytes, got %d", ErrBadFormat, chunk.Offset, len(payload))
	}

	format := &FormatChunk{
		ChunkIndex:     chunk.Index,
		Tag:            binary.LittleEndian.Uint16(payload[0:2]),
		Channels:       binary.LittleEndian.Uint16(payload[2:4]),
		SampleRate:     binary.LittleEndian.Uint32(payload[4:8]),
		AvgBytesPerSec: binary.LittleEndian.Uint32(payload[8:12]),
		BlockAlign:     binary.LittleEndian.Uint16(payload[12:14]),
		BitsPerSample:  binary.LittleEndian.Uint16(payload[14:16]),
		EffectiveTag:   binary.LittleEndian.Uint16(payload[0:2]),
	}
	if len(payload) >= 18 {
		format.CBSize = binary.LittleEndian.Uint16(payload[16:18])
	}
	if format.Tag == FormatExtensible && len(payload) >= 40 {
		format.Extensible = true
		format.ChannelMask = binary.LittleEndian.Uint32(payload[20:24])
		copy(format.SubFormat[:], payload[24:40])
		if tag := extensibleFormatTag(format.SubFormat); tag != 0 {
			format.EffectiveTag = tag
		}
	}
	if format.Channels == 0 || format.SampleRate == 0 {
		return nil, fmt.Errorf("%w at offset %d: channels and sample rate must be non-zero", ErrBadFormat, chunk.Offset)
	}
	return format, nil
}

func extensibleFormatTag(sub [16]byte) uint16 {
	// The WAVEFORMATEXTENSIBLE SubFormat GUID starts with the conventional
	// format code in its first little-endian Data1 field.
	tag := binary.LittleEndian.Uint16(sub[0:2])
	switch tag {
	case FormatPCM, FormatFloat, FormatALaw, FormatMuLaw:
		return tag
	default:
		return 0
	}
}

func parseFact(chunk *Chunk) *FactChunk {
	fact := &FactChunk{ChunkIndex: chunk.Index, Size: chunk.DeclaredSize}
	if len(chunk.Payload) >= 4 {
		fact.SampleCount = binary.LittleEndian.Uint32(chunk.Payload[0:4])
		fact.Valid = chunk.DeclaredSize >= 4
	}
	return fact
}

// ChunkByID returns the first chunk with id.
func (f *File) ChunkByID(id string) *Chunk {
	for _, chunk := range f.Chunks {
		if chunk.ID == id {
			return chunk
		}
	}
	return nil
}
