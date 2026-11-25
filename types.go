package cafmarker

import "time"

// FourCC is a big-endian CAF four-character code.
type FourCC [4]byte

func (c FourCC) String() string {
	b := c
	for i, v := range b {
		if v < 0x20 || v > 0x7e {
			b[i] = '.'
		}
	}
	return string(b[:])
}

// ChunkInfo describes one top-level CAF chunk.
type ChunkInfo struct {
	Type         FourCC `json:"type"`
	HeaderOffset int64  `json:"header_offset"`
	DataOffset   int64  `json:"data_offset"`
	Size         int64  `json:"size"`
	UnknownSize  bool   `json:"unknown_size,omitempty"`
}

// Format is the 'desc' chunk (CAFAudioFormat).
type Format struct {
	ChunkInfo
	SampleRate       float64 `json:"sample_rate"`
	FormatID         FourCC  `json:"format_id"`
	FormatFlags      uint32  `json:"format_flags"`
	BytesPerPacket   uint32  `json:"bytes_per_packet"`
	FramesPerPacket  uint32  `json:"frames_per_packet"`
	ChannelsPerFrame uint32  `json:"channels_per_frame"`
	BitsPerChannel   uint32  `json:"bits_per_channel"`
}

// VariableBytes reports whether packet byte sizes come from 'pakt'.
func (f Format) VariableBytes() bool { return f.BytesPerPacket == 0 }

// VariableFrames reports whether frame counts come from 'pakt'.
func (f Format) VariableFrames() bool { return f.FramesPerPacket == 0 }

// BytesPerFrame returns the declared uncompressed frame size when known.
func (f Format) BytesPerFrame() int64 {
	if f.FormatID.String() == "lpcm" && f.BytesPerPacket > 0 && f.FramesPerPacket == 1 {
		return int64(f.BytesPerPacket)
	}
	if f.ChannelsPerFrame == 0 || f.BitsPerChannel == 0 || f.BitsPerChannel%8 != 0 {
		return 0
	}
	return int64(f.ChannelsPerFrame) * int64(f.BitsPerChannel/8)
}

// PacketTable is the decoded 'pakt' chunk.
type PacketTable struct {
	ChunkInfo
	NumberPackets     int64 `json:"number_packets"`
	NumberValidFrames int64 `json:"number_valid_frames"`
	PrimingFrames     int32 `json:"priming_frames"`
	RemainderFrames   int32 `json:"remainder_frames"`
}

// SMPTETime is the on-disk 8-byte CAF timestamp.
type SMPTETime struct {
	Hours                int8   `json:"hours"`
	Minutes              int8   `json:"minutes"`
	Seconds              int8   `json:"seconds"`
	Frames               int8   `json:"frames"`
	SubFrameSampleOffset uint32 `json:"subframe_sample_offset"`
	Invalid              bool   `json:"invalid"`
}

// Marker is one 28-byte CAFMarker plus its resolved string name.
type Marker struct {
	ChunkOffset   int64     `json:"chunk_offset"`
	IndexInChunk  int       `json:"index_in_chunk"`
	Type          uint32    `json:"type"`
	TypeName      string    `json:"type_name"`
	FramePosition float64   `json:"frame_position"`
	Frame         int64     `json:"frame"`
	FrameValid    bool      `json:"frame_valid"`
	Fractional    bool      `json:"fractional,omitempty"`
	MarkerID      uint32    `json:"marker_id"`
	Name          string    `json:"name,omitempty"`
	SMPTEType     uint32    `json:"smpte_type,omitempty"`
	SMPTETime     SMPTETime `json:"smpte_time"`
	Channel       uint32    `json:"channel"`
}

// DataChunk describes 'data', excluding its four-byte edit count.
type DataChunk struct {
	ChunkInfo
	EditCount          uint32 `json:"edit_count"`
	AudioOffset        int64  `json:"audio_offset"`
	AudioSize          int64  `json:"audio_size"`
	AvailableAudioSize int64  `json:"available_audio_size"`
	Truncated          bool   `json:"truncated,omitempty"`
}

// Issue is a parse, consistency, or bounds warning.
type Issue struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// File is a parsed CAF container and its standard diagnostic chunks.
type File struct {
	Path          string            `json:"path,omitempty"`
	FileVersion   uint16            `json:"file_version"`
	FileFlags     uint16            `json:"file_flags"`
	FileSize      int64             `json:"file_size"`
	Chunks        []ChunkInfo       `json:"chunks"`
	Format        *Format           `json:"format,omitempty"`
	PacketTable   *PacketTable      `json:"packet_table,omitempty"`
	Data          *DataChunk        `json:"data,omitempty"`
	Markers       []Marker          `json:"markers,omitempty"`
	Strings       map[uint32]string `json:"strings,omitempty"`
	Issues        []Issue           `json:"issues,omitempty"`
	packetEntries []packetEntry
}

// Timestamp converts a frame count to seconds using the format sample rate.
func (f *File) Timestamp(frame int64) (time.Duration, bool) {
	if f == nil || f.Format == nil || f.Format.SampleRate <= 0 {
		return 0, false
	}
	seconds := float64(frame) / f.Format.SampleRate
	return time.Duration(seconds * float64(time.Second)), true
}
