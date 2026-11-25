package cafmarker

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
)

var (
	ErrNotCAF             = errors.New("cafmarker: not a CAF file")
	ErrUnsupportedVersion = errors.New("cafmarker: unsupported CAF version")
	ErrMissingFormat      = errors.New("cafmarker: missing desc chunk")
	ErrMissingData        = errors.New("cafmarker: missing data chunk")
	ErrMissingPacketTable = errors.New("cafmarker: missing pakt chunk for variable format")
)

// ParseFile reads and parses a CAF file.
func ParseFile(path string) (*File, error) {
	rs, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	info, err := rs.Stat()
	if err != nil {
		return nil, err
	}
	f, err := ParseReadSeeker(rs, info.Size())
	if f != nil {
		f.Path = path
	}
	return f, err
}

// ParseReadSeeker parses CAF from rs. size is the complete file size.
func ParseReadSeeker(rs io.ReadSeeker, size int64) (*File, error) {
	if size < 0 {
		end, err := rs.Seek(0, io.SeekEnd)
		if err != nil {
			return nil, err
		}
		size = end
		if _, err := rs.Seek(0, io.SeekStart); err != nil {
			return nil, err
		}
	}

	f := &File{FileSize: size, Strings: map[uint32]string{}}
	var fileHeader [8]byte
	if _, err := io.ReadFull(rs, fileHeader[:]); err != nil {
		return nil, fmt.Errorf("read CAF header: %w", err)
	}
	if string(fileHeader[0:4]) != "caff" {
		return nil, ErrNotCAF
	}
	f.FileVersion = binary.BigEndian.Uint16(fileHeader[4:6])
	f.FileFlags = binary.BigEndian.Uint16(fileHeader[6:8])
	if f.FileVersion != 1 {
		f.Issues = append(f.Issues, Issue{
			Code:    "unsupported_version",
			Message: fmt.Sprintf("CAF version %d is not version 1", f.FileVersion),
		})
	}

	offset := int64(8)
	sawData := false
	for offset+12 <= size {
		var header [12]byte
		if _, err := rs.Read(header[:]); err != nil {
			return f, fmt.Errorf("read chunk header at %d: %w", offset, err)
		}
		chunkType := FourCC(header[0:4])
		chunkSize := int64(binary.BigEndian.Uint64(header[4:12]))
		dataOffset := offset + 12
		ci := ChunkInfo{Type: chunkType, HeaderOffset: offset, DataOffset: dataOffset, Size: chunkSize}

		if chunkSize < 0 && chunkType != [4]byte{'d', 'a', 't', 'a'} {
			f.Issues = append(f.Issues, Issue{"negative_chunk_size",
				fmt.Sprintf("%s chunk has invalid negative size %d", chunkType, chunkSize)})
			return f, fmt.Errorf("%w: %s size %d", ErrInvalidChunk, chunkType, chunkSize)
		}

		if chunkType == [4]byte{'d', 'a', 't', 'a'} && chunkSize < 0 {
			if chunkSize != -1 {
				return f, fmt.Errorf("%w: data chunk has invalid negative size %d", ErrInvalidChunk, chunkSize)
			}
			ci.UnknownSize = true
			ci.Size = size - dataOffset
			chunkSize = ci.Size
			if chunkSize < 0 {
				chunkSize = 0
			}
			sawData = true
			dc, err := parseDataChunk(rs, ci, chunkSize, true)
			if err != nil {
				return f, err
			}
			f.Data = dc
			f.Chunks = append(f.Chunks, dc.ChunkInfo)
			return f, f.finalizeParse()
		}

		end, overflow := addInt64(dataOffset, chunkSize)
		if overflow || chunkSize < 0 {
			return f, fmt.Errorf("%w: %s chunk end overflow", ErrInvalidChunk, chunkType)
		}
		available := chunkSize
		truncated := end > size
		if truncated {
			available = size - dataOffset
			if available < 0 {
				available = 0
			}
			f.Issues = append(f.Issues, Issue{"truncated_chunk",
				fmt.Sprintf("%s chunk needs %d bytes but file tail provides %d", chunkType, chunkSize, available)})
		}

		if chunkType == [4]byte{'d', 'a', 't', 'a'} {
			dc, err := parseDataChunk(rs, ci, chunkSize, false)
			if err != nil {
				return f, err
			}
			dc.Truncated = truncated
			if truncated {
				dc.AvailableAudioSize = maxInt64(0, size-(dc.AudioOffset))
			}
			f.Chunks = append(f.Chunks, dc.ChunkInfo)
			if f.Data == nil {
				f.Data = dc
			} else {
				f.addIssue("duplicate_data", "multiple data chunks; using the first")
			}
			sawData = true
			if truncated {
				return f, f.finalizeParse()
			}
			offset = end
			if _, err := rs.Seek(offset, io.SeekStart); err != nil {
				return f, err
			}
			continue
		}

		f.Chunks = append(f.Chunks, ci)

		chunkName := chunkType.String()
		if chunkName != "desc" && chunkName != "pakt" &&
			chunkName != "mark" && chunkName != "strg" {
			offset = end
			if _, err := rs.Seek(offset, io.SeekStart); err != nil {
				return f, err
			}
			continue
		}

		payload := make([]byte, available)
		if _, err := io.ReadFull(rs, payload); err != nil {
			return f, fmt.Errorf("read %s chunk: %w", chunkType, err)
		}

		switch chunkType {
		case FourCC{'d', 'e', 's', 'c'}:
			format, err := parseFormat(ci, payload)
			if err != nil {
				f.addIssue("invalid_desc", err.Error())
				return f, err
			}
			if f.Format != nil {
				f.addIssue("duplicate_desc", "multiple desc chunks; using the first")
			} else {
				f.Format = format
			}
		case FourCC{'p', 'a', 'k', 't'}:
			pt, entries, err := parsePacketTable(ci, payload, f.Format)
			if err != nil {
				f.addIssue("invalid_pakt", err.Error())
				if !errors.Is(err, io.ErrUnexpectedEOF) {
					return f, err
				}
			}
			if f.PacketTable == nil {
				f.PacketTable = pt
				f.packetEntries = entries
			} else {
				f.addIssue("duplicate_pakt", "multiple pakt chunks; using the first")
			}
		case FourCC{'m', 'a', 'r', 'k'}:
			markers, smpteType, err := parseMarkers(payload)
			for i := range markers {
				markers[i].ChunkOffset = dataOffset + 8 + int64(i)*28
				markers[i].SMPTEType = smpteType
			}
			f.Markers = append(f.Markers, markers...)
			if err != nil {
				f.addIssue("invalid_mark", err.Error())
			}
		case FourCC{'s', 't', 'r', 'g'}:
			if err := parseStrings(payload, f.Strings); err != nil {
				f.addIssue("invalid_strg", err.Error())
			}
		}

		if truncated {
			return f, f.finalizeParse()
		}
		offset = end
		if _, err := rs.Seek(offset, io.SeekStart); err != nil {
			return f, err
		}
	}

	if !sawData {
		f.addIssue("missing_data", "required data chunk was not found")
	}
	if offset < size {
		f.addIssue("trailing_bytes", fmt.Sprintf("%d trailing byte(s) after the last parseable chunk boundary", size-offset))
	}
	return f, f.finalizeParse()
}

func (f *File) finalizeParse() error {
	if f.Format == nil {
		return ErrMissingFormat
	}
	if f.Data == nil {
		f.addIssue("missing_data", "required data chunk was not found")
		return ErrMissingData
	}
	for i := range f.Markers {
		if name, ok := f.Strings[f.Markers[i].MarkerID]; ok {
			f.Markers[i].Name = name
		}
	}
	if (f.Format.VariableBytes() || f.Format.VariableFrames()) && f.PacketTable == nil {
		f.addIssue("missing_packet_table", "format has variable packet dimensions but no pakt chunk was parsed")
		return ErrMissingPacketTable
	}
	return nil
}

func (f *File) addIssue(code, message string) {
	f.Issues = append(f.Issues, Issue{Code: code, Message: message})
}

var ErrInvalidChunk = errors.New("cafmarker: invalid chunk")

func addInt64(a, b int64) (int64, bool) {
	if b < 0 {
		if a < math.MinInt64-b {
			return 0, true
		}
	} else if a > math.MaxInt64-b {
		return 0, true
	}
	return a + b, false
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func parseDataChunk(rs io.ReadSeeker, ci ChunkInfo, chunkPayloadSize int64, unknown bool) (*DataChunk, error) {
	if chunkPayloadSize < 4 {
		return nil, fmt.Errorf("%w: data chunk too short for edit count", ErrInvalidChunk)
	}
	var edit [4]byte
	if _, err := rs.Read(edit[:]); err != nil {
		return nil, fmt.Errorf("read data edit count: %w", err)
	}
	audioOffset := ci.DataOffset + 4
	audioSize := chunkPayloadSize - 4
	dc := &DataChunk{
		ChunkInfo:          ci,
		EditCount:          binary.BigEndian.Uint32(edit[:]),
		AudioOffset:        audioOffset,
		AudioSize:          audioSize,
		AvailableAudioSize: audioSize,
	}
	if unknown {
		dc.UnknownSize = true
	}
	return dc, nil
}
