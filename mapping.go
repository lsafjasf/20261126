package cafmarker

import (
	"fmt"
	"math"
	"sort"
)

// Packet is the resolved sample/frame/byte mapping for one CAF packet.
type Packet struct {
	Index          int      `json:"index"`
	FrameStart     int64    `json:"frame_start"`
	FrameCount     int64    `json:"frame_count"`
	FrameEnd       int64    `json:"frame_end"`
	ByteOffset     int64    `json:"byte_offset"`
	ByteSize       int64    `json:"byte_size"`
	AvailableBytes int64    `json:"available_bytes"`
	MissingBytes   int64    `json:"missing_bytes,omitempty"`
	Flags          []string `json:"flags,omitempty"`
}

// PacketMap contains contiguous packet boundaries and coordinate metadata.
type PacketMap struct {
	Packets                  []Packet        `json:"packets"`
	PacketCountDeclared      int64           `json:"packet_count_declared"`
	PacketCountMapped        int64           `json:"packet_count_mapped"`
	PacketsTruncated         bool            `json:"packets_truncated,omitempty"`
	Constant                 *ConstantLayout `json:"constant,omitempty"`
	CodedFrameCount          int64           `json:"coded_frame_count"`
	DeclaredValidFrameCount  int64           `json:"declared_valid_frame_count,omitempty"`
	PrimingFrames            int64           `json:"priming_frames"`
	RemainderFrames          int64           `json:"remainder_frames"`
	AudibleFrameCount        int64           `json:"audible_frame_count"`
	AudioByteSize            int64           `json:"audio_byte_size"`
	AvailableAudioBytes      int64           `json:"available_audio_bytes"`
	MappedPacketBytes        int64           `json:"mapped_packet_bytes"`
	TrailingUntabulatedBytes int64           `json:"trailing_untabulated_bytes,omitempty"`
	Issues                   []Issue         `json:"issues,omitempty"`
}

// ConstantLayout describes a format where packet byte and frame sizes are fixed.
type ConstantLayout struct {
	BytesPerPacket  int64 `json:"bytes_per_packet"`
	FramesPerPacket int64 `json:"frames_per_packet"`
	PacketCount     int64 `json:"packet_count"`
}

const maxMaterializedPackets = 10000

// FrameLocation is the result of mapping one CAF frame number.
type FrameLocation struct {
	Frame               int64    `json:"frame"`
	Valid               bool     `json:"valid"`
	PacketIndex         int      `json:"packet_index"`
	FrameOffsetInPacket int64    `json:"frame_offset_in_packet,omitempty"`
	PacketByteOffset    int64    `json:"packet_byte_offset,omitempty"`
	ByteOffsetInPacket  int64    `json:"byte_offset_in_packet,omitempty"`
	AudioByteOffset     int64    `json:"audio_byte_offset,omitempty"`
	FileByteOffset      int64    `json:"file_byte_offset,omitempty"`
	Boundary            bool     `json:"boundary,omitempty"`
	BoundaryAfterPacket int      `json:"boundary_after_packet"`
	AudibleFrame        int64    `json:"audible_frame,omitempty"`
	TimeSeconds         float64  `json:"time_seconds,omitempty"`
	Status              []string `json:"status,omitempty"`
}

// BuildMap resolves packet table entries against desc and data chunks.
func (f *File) BuildMap() (*PacketMap, error) {
	if f.Format == nil {
		return nil, ErrMissingFormat
	}
	if f.Data == nil {
		return nil, ErrMissingData
	}

	m := &PacketMap{}
	availableAudio := f.Data.AvailableAudioSize
	if availableAudio == 0 && !f.Data.Truncated {
		availableAudio = f.Data.AudioSize
	}
	m.AudioByteSize = f.Data.AudioSize
	m.AvailableAudioBytes = availableAudio

	if f.PacketTable != nil {
		m.PacketCountDeclared = f.PacketTable.NumberPackets
		m.DeclaredValidFrameCount = f.PacketTable.NumberValidFrames
		m.PrimingFrames = int64(f.PacketTable.PrimingFrames)
		m.RemainderFrames = int64(f.PacketTable.RemainderFrames)
	}

	if (f.Format.VariableBytes() || f.Format.VariableFrames()) && f.PacketTable == nil {
		m.Issues = append(m.Issues, Issue{"missing_packet_table", "variable packet dimensions require a pakt chunk"})
		return m, ErrMissingPacketTable
	}

	descriptors := f.packetEntries
	if len(descriptors) == 0 && !f.Format.VariableBytes() && !f.Format.VariableFrames() {
		if f.Format.BytesPerPacket == 0 || f.Format.FramesPerPacket == 0 {
			m.Issues = append(m.Issues, Issue{"zero_constant_packet_dimension", "constant packet format has zero bytes-per-packet or frames-per-packet"})
			return m, ErrInvalidChunk
		}
		bpp := int64(f.Format.BytesPerPacket)
		fpp := int64(f.Format.FramesPerPacket)
		dataPackets := availableAudio / bpp
		if availableAudio%bpp != 0 {
			dataPackets++
		}
		count := m.PacketCountDeclared
		if count <= 0 {
			count = dataPackets
		}
		m.Constant = &ConstantLayout{BytesPerPacket: bpp, FramesPerPacket: fpp, PacketCount: count}
		if count <= maxMaterializedPackets {
			descriptors = make([]packetEntry, 0, count)
			for i := int64(0); i < count; i++ {
				descriptors = append(descriptors, packetEntry{Size: bpp, Frames: fpp})
			}
		}
	}

	if m.PacketCountDeclared == 0 {
		m.PacketCountDeclared = int64(len(descriptors))
	}
	if int64(len(descriptors)) < m.PacketCountDeclared && (f.Format.VariableBytes() || f.Format.VariableFrames()) {
		m.Issues = append(m.Issues, Issue{
			Code: "packet_table_short",
			Message: fmt.Sprintf("pakt declares %d packets but only %d entries can be decoded",
				m.PacketCountDeclared, len(descriptors)),
		})
	}

	var frameCursor, byteCursor int64
	m.Packets = make([]Packet, 0, len(descriptors))
	for i, descriptor := range descriptors {
		size := descriptor.Size
		if size == 0 && !f.Format.VariableBytes() {
			size = int64(f.Format.BytesPerPacket)
		}
		frames := descriptor.Frames
		if frames == 0 && !f.Format.VariableFrames() {
			frames = int64(f.Format.FramesPerPacket)
		}

		p := Packet{
			Index:      i,
			FrameStart: frameCursor,
			FrameCount: frames,
			FrameEnd:   frameCursor + frames,
			ByteOffset: byteCursor,
			ByteSize:   size,
		}
		if size < 0 || frames < 0 {
			p.Flags = append(p.Flags, "negative_packet_descriptor")
			size, frames = max64(size, 0), max64(frames, 0)
			p.ByteSize, p.FrameCount = size, frames
			p.FrameEnd = p.FrameStart + frames
		}
		audioRemaining := availableAudio - p.ByteOffset
		if audioRemaining > 0 {
			p.AvailableBytes = min64(size, audioRemaining)
		}
		p.MissingBytes = size - p.AvailableBytes
		if p.MissingBytes > 0 {
			p.Flags = append(p.Flags, "short_byte_tail")
		}
		if size == 0 {
			p.Flags = append(p.Flags, "zero_byte_packet")
		}
		if frames == 0 {
			p.Flags = append(p.Flags, "zero_frame_packet")
		}
		m.Packets = append(m.Packets, p)
		frameCursor = p.FrameEnd
		byteCursor += size
	}

	if m.Constant != nil && len(descriptors) == 0 {
		var frameOverflow, byteOverflow bool
		frameCursor, frameOverflow = mulNonNegative(m.Constant.PacketCount, m.Constant.FramesPerPacket)
		byteCursor, byteOverflow = mulNonNegative(m.Constant.PacketCount, m.Constant.BytesPerPacket)
		if frameOverflow || byteOverflow {
			m.Issues = append(m.Issues, Issue{"layout_overflow", "constant packet count/size product overflows int64"})
		}
		m.Packets = []Packet{}
		m.PacketsTruncated = m.Constant.PacketCount > 0
	}
	m.PacketCountMapped = packetCount(m, len(m.Packets))
	m.CodedFrameCount = frameCursor
	m.MappedPacketBytes = byteCursor
	m.AudibleFrameCount = max64(0, m.CodedFrameCount-m.PrimingFrames-m.RemainderFrames)

	if f.PacketTable != nil && m.DeclaredValidFrameCount > 0 {
		expected := m.AudibleFrameCount
		if expected != m.DeclaredValidFrameCount {
			m.Issues = append(m.Issues, Issue{
				Code: "valid_frame_count_mismatch",
				Message: fmt.Sprintf(
					"sum(packet frames)=%d - priming=%d - remainder=%d gives %d, but pakt.mNumberValidFrames=%d",
					m.CodedFrameCount, m.PrimingFrames, m.RemainderFrames, expected, m.DeclaredValidFrameCount),
			})
		}
	}
	if byteCursor > availableAudio {
		m.Issues = append(m.Issues, Issue{
			Code: "packet_data_short",
			Message: fmt.Sprintf("packet table needs %d audio bytes but data chunk provides %d; shortfall=%d",
				byteCursor, availableAudio, byteCursor-availableAudio),
		})
	} else if availableAudio > byteCursor && (m.Constant != nil || int64(len(m.Packets)) == m.PacketCountDeclared) {
		m.TrailingUntabulatedBytes = availableAudio - byteCursor
		m.Issues = append(m.Issues, Issue{
			Code: "untabulated_audio_tail",
			Message: fmt.Sprintf("data chunk has %d audio bytes after the last packet-table packet",
				m.TrailingUntabulatedBytes),
		})
	}
	return m, nil
}

func packetCount(m *PacketMap, materialized int) int64 {
	if m.Constant != nil {
		return m.Constant.PacketCount
	}
	return int64(materialized)
}

// PacketAt returns packet i, computing constant-layout packets on demand.
func (m *PacketMap) PacketAt(i int) (Packet, bool) {
	if m == nil || i < 0 {
		return Packet{}, false
	}
	if i < len(m.Packets) {
		return m.Packets[i], true
	}
	if m.Constant == nil || int64(i) >= m.Constant.PacketCount {
		return Packet{}, false
	}
	bpp := m.Constant.BytesPerPacket
	fpp := m.Constant.FramesPerPacket
	p := Packet{
		Index:      i,
		FrameStart: int64(i) * fpp,
		FrameCount: fpp,
		FrameEnd:   int64(i+1) * fpp,
		ByteOffset: int64(i) * bpp,
		ByteSize:   bpp,
	}
	if m.AvailableAudioBytes > p.ByteOffset {
		p.AvailableBytes = min64(p.ByteSize, m.AvailableAudioBytes-p.ByteOffset)
	}
	p.MissingBytes = p.ByteSize - p.AvailableBytes
	if p.MissingBytes > 0 {
		p.Flags = append(p.Flags, "short_byte_tail")
	}
	return p, true
}

// LocateFrame maps a CAF frame number to its packet and byte coordinates.
func (m *PacketMap) LocateFrame(frame int64, f *File) FrameLocation {
	loc := FrameLocation{Frame: frame, PacketIndex: -1, BoundaryAfterPacket: -1}
	if m == nil || f == nil {
		loc.Status = append(loc.Status, "missing_map")
		return loc
	}
	if frame < 0 {
		loc.Status = append(loc.Status, "negative_frame")
		return loc
	}

	if m.Constant != nil && m.Constant.FramesPerPacket > 0 {
		fpp := m.Constant.FramesPerPacket
		if frame < m.CodedFrameCount {
			index64 := frame / fpp
			index := int(index64)
			if p, ok := m.PacketAt(index); ok {
				loc.Valid = true
				loc.PacketIndex = index
				loc.FrameOffsetInPacket = frame - p.FrameStart
				loc.PacketByteOffset = p.ByteOffset
				loc.AudioByteOffset = p.ByteOffset
				loc.Boundary = loc.FrameOffsetInPacket == 0 && index > 0
				if loc.Boundary {
					loc.BoundaryAfterPacket = index - 1
				}
				loc.Status = append(loc.Status, p.Flags...)
				if bpf := f.Format.BytesPerFrame(); bpf > 0 {
					inner := loc.FrameOffsetInPacket * bpf
					if inner <= p.ByteSize {
						loc.ByteOffsetInPacket = inner
						loc.AudioByteOffset = p.ByteOffset + inner
					}
				}
				if f.Data != nil {
					loc.FileByteOffset = f.Data.AudioOffset + loc.AudioByteOffset
				}
			}
		}
	}

	if !loc.Valid {
		i := sort.Search(len(m.Packets), func(i int) bool {
			return m.Packets[i].FrameEnd > frame
		})
		if i < len(m.Packets) {
			p := m.Packets[i]
			if frame >= p.FrameStart && frame < p.FrameEnd {
				loc.Valid = true
				loc.PacketIndex = i
				loc.FrameOffsetInPacket = frame - p.FrameStart
				loc.PacketByteOffset = p.ByteOffset
				loc.AudioByteOffset = p.ByteOffset
				for _, flag := range p.Flags {
					loc.Status = append(loc.Status, flag)
				}
				if frame == p.FrameStart && i > 0 && m.Packets[i-1].FrameEnd == frame {
					loc.Boundary = true
					loc.BoundaryAfterPacket = i - 1
				}
				if bpf := f.Format.BytesPerFrame(); bpf > 0 {
					inner := loc.FrameOffsetInPacket * bpf
					if inner <= p.ByteSize {
						loc.ByteOffsetInPacket = inner
						loc.AudioByteOffset = p.ByteOffset + inner
					}
				}
				if f.Data != nil {
					loc.FileByteOffset = f.Data.AudioOffset + loc.AudioByteOffset
				}
			}
		}
	}

	if !loc.Valid {
		totalPackets := m.PacketCountMapped
		if totalPackets > 0 && frame == m.CodedFrameCount {
			loc.Boundary = true
			loc.BoundaryAfterPacket = int(totalPackets) - 1
			loc.Status = append(loc.Status, "end_of_packets")
		} else if m.Constant == nil && int64(len(m.Packets)) < m.PacketCountDeclared {
			loc.Status = append(loc.Status, "packet_table_truncated")
		} else if frame > m.CodedFrameCount {
			loc.Status = append(loc.Status, "beyond_coded_frames")
		} else {
			loc.Status = append(loc.Status, "unmapped_frame")
		}
	}

	if m.PrimingFrames > 0 && frame < m.PrimingFrames {
		loc.Status = append(loc.Status, "priming_frame")
	}
	audibleEnd := max64(0, m.CodedFrameCount-m.RemainderFrames)
	if m.RemainderFrames > 0 && frame >= audibleEnd {
		loc.Status = append(loc.Status, "remainder_frame")
	}
	if frame >= m.PrimingFrames && frame < audibleEnd {
		loc.AudibleFrame = frame - m.PrimingFrames
	}
	if f.Format != nil && f.Format.SampleRate > 0 {
		loc.TimeSeconds = float64(frame) / f.Format.SampleRate
	}
	return loc
}

// LocateMarker is a convenience wrapper for marker frame positions.
func (f *File) LocateMarker(marker Marker) (*PacketMap, FrameLocation, error) {
	m, err := f.BuildMap()
	if err != nil && m == nil {
		return nil, FrameLocation{}, err
	}
	frame := marker.Frame
	if !marker.FrameValid {
		frame = -1
	}
	loc := m.LocateFrame(frame, f)
	return m, loc, err
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func mulNonNegative(a, b int64) (int64, bool) {
	if a == 0 || b == 0 {
		return 0, false
	}
	if a > math.MaxInt64/b {
		return math.MaxInt64, true
	}
	return a * b, false
}
