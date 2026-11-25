package aiffmeta

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
)

const (
	minFormSize      = 4
	maxMetadataChunk = int64(64 << 20)
)

var ErrNotAIFF = errors.New("not an AIFF/AIFC file")

// ReadFile opens path and parses its metadata. The returned File keeps the
// file open for later streaming; callers should defer File.Close.
func ReadFile(path string) (*File, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}
	parsed, err := Parse(file, info.Size())
	if err != nil {
		file.Close()
		return nil, err
	}
	parsed.Path = path
	parsed.closer = file
	return parsed, nil
}

// ParseBytes parses an in-memory AIFF/AIFC image.
func ParseBytes(data []byte) (*File, error) {
	return Parse(bytes.NewReader(data), int64(len(data)))
}

// Parse walks the FORM container and parses COMM, SSND and textual metadata.
// Truncated payloads and trailing bytes are reported as warnings where enough
// data exists to continue; a malformed FORM header is a fatal error.
func Parse(r io.ReaderAt, size int64) (*File, error) {
	if r == nil {
		return nil, errors.New("nil reader")
	}
	if size < 12 {
		return nil, fmt.Errorf("%w: file is %d bytes, need at least 12", ErrNotAIFF, size)
	}

	header := make([]byte, 12)
	if _, err := r.ReadAt(header, 0); err != nil {
		return nil, fmt.Errorf("read FORM header: %w", err)
	}
	if string(header[0:4]) != "FORM" {
		return nil, fmt.Errorf("%w: leading ID is %q, want FORM", ErrNotAIFF, header[0:4])
	}
	formSize := binary.BigEndian.Uint32(header[4:8])
	formType := string(header[8:12])
	if formType != string(FormatAIFF) && formType != string(FormatAIFC) {
		return nil, fmt.Errorf("%w: FORM type is %q", ErrNotAIFF, formType)
	}
	if formSize < minFormSize {
		return nil, fmt.Errorf("%w: FORM size %d is smaller than its type field", ErrNotAIFF, formSize)
	}

	declaredEnd := int64(8) + int64(formSize)
	walkEnd := declaredEnd
	f := &File{
		Size:          size,
		Format:        Format(formType),
		FormSize:      formSize,
		FormDataStart: 12,
		FormEnd:       declaredEnd,
		source:        r,
	}
	if declaredEnd > size {
		f.FormTruncated = true
		walkEnd = size
		f.addIssue(4, "FORM", "declares end at %d, file ends at %d", declaredEnd, size)
	} else if size > declaredEnd {
		f.TrailingBytes = size - declaredEnd
		f.addIssue(declaredEnd, "FORM", "%d byte(s) after the declared FORM end", f.TrailingBytes)
	}

	f.Chunks = walkChunks(f, r, walkEnd)
	for i := range f.Chunks {
		chunk := &f.Chunks[i]
		switch chunk.Kind {
		case KindCommon:
			if f.COMM != nil {
				f.addIssue(chunk.HeaderOffset, chunk.ID, "duplicate COMM chunk ignored")
				continue
			}
			comm, err := parseCOMM(r, *chunk, f.Format)
			if err != nil {
				f.addIssue(chunk.HeaderOffset, chunk.ID, "parse COMM: %v", err)
				if comm == nil {
					continue
				}
			}
			if !finiteRate(comm.SampleRate) {
				f.addIssue(chunk.HeaderOffset, chunk.ID,
					"sample rate is not finite; raw extended value is %s", comm.ExtendedRateHex)
				comm.SampleRate = 0
			}
			if comm.Channels == 0 {
				f.addIssue(chunk.HeaderOffset, chunk.ID, "channel count is zero")
			}
			if comm.SampleSize == 0 || comm.SampleSize > 32 {
				f.addIssue(chunk.HeaderOffset, chunk.ID,
					"sample size %d is outside the AIFF range 1..32", comm.SampleSize)
			}
			f.COMM = comm
		case KindAudio:
			if f.SSND != nil {
				f.addIssue(chunk.HeaderOffset, chunk.ID, "duplicate SSND chunk ignored")
				continue
			}
			ssnd, err := parseSSND(r, *chunk)
			if err != nil {
				f.addIssue(chunk.HeaderOffset, chunk.ID, "parse SSND: %v", err)
				if ssnd == nil {
					continue
				}
			}
			f.SSND = ssnd
		case KindText, KindComments, KindMarkers:
			if err := parseTextChunk(r, *chunk, &f.Text, f); err != nil {
				f.addIssue(chunk.HeaderOffset, chunk.ID, "parse %s: %v", chunk.ID, err)
			}
		}
	}
	checkSoundLayout(f)
	return f, nil
}

func walkChunks(f *File, r io.ReaderAt, walkEnd int64) []Chunk {
	var chunks []Chunk
	pos := int64(12)
	for pos < walkEnd {
		if walkEnd-pos < 8 {
			f.addIssue(pos, "", "%d trailing byte(s) do not form a chunk header", walkEnd-pos)
			break
		}
		header := make([]byte, 8)
		if _, err := r.ReadAt(header, pos); err != nil {
			f.addIssue(pos, "", "read chunk header: %v", err)
			break
		}
		id := string(header[0:4])
		declaredSize := binary.BigEndian.Uint32(header[4:8])
		dataOffset := pos + 8
		dataEnd := dataOffset + int64(declaredSize)
		paddedEnd := dataEnd
		if declaredSize&1 != 0 {
			paddedEnd++
		}
		storedEnd := dataEnd
		truncated := false
		if storedEnd > walkEnd {
			storedEnd = walkEnd
			truncated = true
			f.addIssue(pos, id, "declares %d payload byte(s), only %d available",
				declaredSize, max64(0, walkEnd-dataOffset))
		}
		next := paddedEnd
		if next > walkEnd {
			next = walkEnd
		}
		chunk := Chunk{
			Index:        len(chunks),
			ID:           id,
			Kind:         classifyChunk(id),
			HeaderOffset: pos,
			DataOffset:   dataOffset,
			Size:         declaredSize,
			StoredSize:   storedEnd - dataOffset,
			DataEnd:      dataEnd,
			PaddedEnd:    paddedEnd,
			NextOffset:   next,
			Truncated:    truncated,
		}
		if chunk.StoredSize < 0 {
			chunk.StoredSize = 0
		}
		chunks = append(chunks, chunk)
		if truncated {
			break
		}
		pos = next
	}
	return chunks
}

func classifyChunk(id string) ChunkKind {
	switch id {
	case "COMM":
		return KindCommon
	case "SSND":
		return KindAudio
	case "COMT":
		return KindComments
	case "MARK":
		return KindMarkers
	case "NAME", "AUTH", "ANNO", "(c) ":
		return KindText
	case "FVER":
		return KindFormat
	case "INST":
		return KindInstrument
	default:
		return KindUnknown
	}
}

func readPayload(r io.ReaderAt, chunk Chunk) ([]byte, error) {
	if chunk.StoredSize > maxMetadataChunk {
		return nil, fmt.Errorf("payload is %d bytes (metadata limit %d)", chunk.StoredSize, maxMetadataChunk)
	}
	if chunk.StoredSize > int64(^uint(0)>>1) {
		return nil, errors.New("payload too large for this platform")
	}
	buf := make([]byte, int(chunk.StoredSize))
	if len(buf) == 0 {
		return buf, nil
	}
	if _, err := r.ReadAt(buf, chunk.DataOffset); err != nil {
		return nil, err
	}
	return buf, nil
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
