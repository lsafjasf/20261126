package aiffmeta

import (
	"fmt"
	"io"
	"math"
	"time"
)

// Format is the form type stored at byte 8 of an AIFF container.
type Format string

const (
	FormatAIFF Format = "AIFF"
	FormatAIFC Format = "AIFC"
)

// ChunkKind is a human-oriented classification used by reports.
type ChunkKind string

const (
	KindCommon     ChunkKind = "common"
	KindAudio      ChunkKind = "audio"
	KindText       ChunkKind = "text"
	KindComments   ChunkKind = "comments"
	KindMarkers    ChunkKind = "markers"
	KindFormat     ChunkKind = "format"
	KindInstrument ChunkKind = "instrument"
	KindUnknown    ChunkKind = "unknown"
)

// Chunk describes one top-level IFF chunk. Offsets are absolute byte
// positions in the file. Size is the declared payload size and does not
// include the 8-byte chunk header or the even-byte padding.
type Chunk struct {
	Index        int       `json:"index"`
	ID           string    `json:"id"`
	Kind         ChunkKind `json:"kind"`
	HeaderOffset int64     `json:"header_offset"`
	DataOffset   int64     `json:"data_offset"`
	Size         uint32    `json:"size"`
	StoredSize   int64     `json:"stored_size"`
	DataEnd      int64     `json:"data_end"`
	PaddedEnd    int64     `json:"padded_end"`
	NextOffset   int64     `json:"next_offset"`
	Truncated    bool      `json:"truncated,omitempty"`
}

func (c Chunk) HeaderEnd() int64 {
	return c.HeaderOffset + 8
}

func (c Chunk) PaddedSize() int64 {
	if c.Size&1 == 0 {
		return int64(c.Size)
	}
	return int64(c.Size) + 1
}

// Issue is a non-fatal format observation. Fatal I/O or container errors are
// returned as errors instead.
type Issue struct {
	Severity string `json:"severity"`
	Offset   int64  `json:"offset,omitempty"`
	Chunk    string `json:"chunk,omitempty"`
	Message  string `json:"message"`
}

// COMM is the parsed common chunk.
type COMM struct {
	ChunkIndex      int     `json:"chunk_index"`
	Channels        uint16  `json:"channels"`
	SampleFrames    uint32  `json:"sample_frames"`
	SampleSize      uint16  `json:"sample_size"`
	SampleRate      float64 `json:"sample_rate"`
	ExtendedRateHex string  `json:"extended_rate_hex"`
	CompressionType string  `json:"compression_type,omitempty"`
	CompressionName string  `json:"compression_name,omitempty"`
	IsAIFC          bool    `json:"is_aifc"`
}

// BytesPerFrame reports the uncompressed storage size for one sample frame.
// For AIFC compression types other than NONE this value may be meaningless.
func (c COMM) BytesPerFrame() uint32 {
	if c.Channels == 0 {
		return 0
	}
	bytesPerSample := (uint32(c.SampleSize) + 7) / 8
	return uint32(c.Channels) * bytesPerSample
}

// ExpectedAudioBytes returns the payload size implied by COMM. It is only
// authoritative for uncompressed AIFF/AIFC data.
func (c COMM) ExpectedAudioBytes() (int64, bool) {
	if c.SampleSize == 0 || c.SampleSize > 32 || c.Channels == 0 {
		return 0, false
	}
	if c.IsAIFC && c.CompressionType != "" &&
		c.CompressionType != "NONE" && c.CompressionType != "twos" &&
		c.CompressionType != "sowt" {
		return 0, false
	}
	return int64(c.SampleFrames) * int64(c.BytesPerFrame()), true
}

// SSND is the parsed sound-data chunk. AudioOffset points after the 8-byte
// SSND parameter block and after the chunk's own data offset.
type SSND struct {
	ChunkIndex  int    `json:"chunk_index"`
	Offset      uint32 `json:"offset"`
	BlockSize   uint32 `json:"block_size"`
	AudioOffset int64  `json:"audio_offset"`
	AudioSize   int64  `json:"audio_size"`
}

// Comment is one entry in a COMT chunk. TimeStamp is seconds since
// 1904-01-01 UTC, as defined by AIFF.
type Comment struct {
	TimeStamp uint32 `json:"timestamp"`
	MarkerID  int16  `json:"marker_id"`
	Text      string `json:"text"`
}

func (c Comment) Time() time.Time {
	const epochOffset = int64(2082844800) // 1904-01-01 to 1970-01-01
	return time.Unix(int64(c.TimeStamp)-epochOffset, 0).UTC()
}

// Marker is one entry in a MARK chunk. Position is a sample-frame number.
type Marker struct {
	ID       int16  `json:"id"`
	Position uint32 `json:"position"`
	Name     string `json:"name"`
}

// TextChunk retains a text-bearing chunk even when it is one of the duplicate
// or less common textual chunk types.
type TextChunk struct {
	ChunkIndex int    `json:"chunk_index"`
	ID         string `json:"id"`
	DataOffset int64  `json:"data_offset"`
	Text       string `json:"text"`
}

// TextMetadata collects the textual chunks defined by AIFF plus MARK/COMT
// entries. Unknown binary chunks are deliberately not interpreted as text.
type TextMetadata struct {
	Name        string      `json:"name,omitempty"`
	Author      string      `json:"author,omitempty"`
	Copyright   string      `json:"copyright,omitempty"`
	Annotations []string    `json:"annotations,omitempty"`
	Comments    []Comment   `json:"comments,omitempty"`
	Markers     []Marker    `json:"markers,omitempty"`
	Chunks      []TextChunk `json:"chunks,omitempty"`
}

// File is the result of walking an AIFF/AIFC container. The source is kept
// privately so audio can be streamed without loading the whole file.
type File struct {
	Path          string       `json:"path,omitempty"`
	Size          int64        `json:"size"`
	Format        Format       `json:"format"`
	FormSize      uint32       `json:"form_size"`
	FormDataStart int64        `json:"form_data_start"`
	FormEnd       int64        `json:"form_end"`
	FormTruncated bool         `json:"form_truncated,omitempty"`
	TrailingBytes int64        `json:"trailing_bytes,omitempty"`
	Chunks        []Chunk      `json:"chunks"`
	COMM          *COMM        `json:"comm,omitempty"`
	SSND          *SSND        `json:"ssnd,omitempty"`
	Text          TextMetadata `json:"text"`
	Warnings      []Issue      `json:"warnings,omitempty"`

	source io.ReaderAt
	closer io.Closer
}

// Close releases a source opened by ReadFile. It is safe to call repeatedly.
func (f *File) Close() error {
	if f == nil || f.closer == nil {
		return nil
	}
	err := f.closer.Close()
	f.closer = nil
	return err
}

func (f *File) addIssue(offset int64, chunk, format string, args ...any) {
	f.Warnings = append(f.Warnings, Issue{
		Severity: "warning",
		Offset:   offset,
		Chunk:    chunk,
		Message:  fmt.Sprintf(format, args...),
	})
}

func finiteRate(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}
