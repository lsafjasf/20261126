package wavfix

import (
	"errors"
)

const (
	FormatPCM        uint16 = 0x0001
	FormatALaw       uint16 = 0x0006
	FormatMuLaw      uint16 = 0x0007
	FormatFloat      uint16 = 0x0003
	FormatExtensible uint16 = 0xFFFE
)

var (
	ErrNotRIFF      = errors.New("wavfix: missing RIFF/WAVE signature")
	ErrTruncated    = errors.New("wavfix: truncated RIFF chunk")
	ErrNoFormat     = errors.New("wavfix: missing fmt chunk")
	ErrNoData       = errors.New("wavfix: missing data chunk")
	ErrBadFormat    = errors.New("wavfix: invalid fmt chunk")
	ErrMisaligned   = errors.New("wavfix: data size is not a whole number of sample frames")
	ErrUnsupported  = errors.New("wavfix: format does not have a fixed G.711 frame size")
	ErrFactOverflow = errors.New("wavfix: sample count does not fit in fact chunk")
)

// Chunk is one raw RIFF chunk. Payload excludes the eight-byte chunk header.
// Raw includes the source bytes consumed for the chunk and its alignment pad
// when that pad is present in the source.
type Chunk struct {
	Index        int
	ID           string
	Offset       int
	DeclaredSize uint32
	DataOffset   int
	NextOffset   int
	Padding      int
	Truncated    bool
	Payload      []byte
	Raw          []byte
}

// FormatChunk is the parsed WAVE fmt chunk.
type FormatChunk struct {
	ChunkIndex     int
	Tag            uint16
	Channels       uint16
	SampleRate     uint32
	AvgBytesPerSec uint32
	BlockAlign     uint16
	BitsPerSample  uint16
	CBSize         uint16
	EffectiveTag   uint16
	Extensible     bool
	ChannelMask    uint32
	SubFormat      [16]byte
}

// FactChunk is the parsed fact chunk. WAVE defines dwSampleLength as the
// number of sample frames, i.e. samples per channel on the timeline.
type FactChunk struct {
	ChunkIndex  int
	Size        uint32
	SampleCount uint32
	Valid       bool
}

// DataChunk identifies the audio payload. Payload is never modified by the
// container repair code.
type DataChunk struct {
	ChunkIndex int
	Offset     int
	Size       uint32
	Payload    []byte
}

// Issue describes a non-fatal inconsistency found while parsing or repairing.
type Issue struct {
	Code    string `json:"code"`
	Chunk   string `json:"chunk,omitempty"`
	Offset  int    `json:"offset,omitempty"`
	Message string `json:"message"`
}

// Report contains the container relationships used to validate and repair a
// file. Durations are in seconds.
type Report struct {
	InputSize             int      `json:"inputSize"`
	OutputSize            int      `json:"outputSize,omitempty"`
	RIFFDeclaredSize      uint32   `json:"riffDeclaredSize"`
	RIFFActualSize        int      `json:"riffActualSize"`
	FormatTag             uint16   `json:"formatTag"`
	EffectiveFormatTag    uint16   `json:"effectiveFormatTag"`
	Channels              uint16   `json:"channels"`
	SampleRate            uint32   `json:"sampleRate"`
	BlockAlign            uint16   `json:"blockAlign"`
	BitsPerSample         uint16   `json:"bitsPerSample"`
	DataBytes             int      `json:"dataBytes"`
	FrameBytes            int      `json:"frameBytes"`
	DataFrames            uint64   `json:"dataFrames"`
	DataRemainderBytes    int      `json:"dataRemainderBytes"`
	FactPresent           bool     `json:"factPresent"`
	FactDeclaredCount     uint32   `json:"factDeclaredCount,omitempty"`
	FactValid             bool     `json:"factValid"`
	RepairedFactCount     uint32   `json:"repairedFactCount,omitempty"`
	FactChanged           bool     `json:"factChanged"`
	FactInserted          bool     `json:"factInserted"`
	DurationFromFact      float64  `json:"durationFromFactSeconds,omitempty"`
	DurationFromData      float64  `json:"durationFromDataSeconds"`
	NaiveByteCountSeconds float64  `json:"naiveByteCountAsFramesSeconds"`
	FloatValues           int      `json:"floatValues"`
	FloatDurationSeconds  float64  `json:"floatDurationSeconds"`
	AudioSHA256           string   `json:"audioSha256"`
	UnknownChunks         []string `json:"unknownChunks"`
	Issues                []Issue  `json:"issues,omitempty"`
}

// File is a parsed WAV/RIFF file. The original byte slice remains the backing
// store for chunk payloads and raw chunk bytes.
type File struct {
	Raw      []byte
	RIFFSize uint32
	Chunks   []*Chunk
	Format   *FormatChunk
	Fact     *FactChunk
	Data     *DataChunk
	Trailing []byte
	Reports  []Issue
}
