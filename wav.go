package channelmap

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
)

const (
	FormatPCM        uint16 = 0x0001
	FormatIEEEFloat  uint16 = 0x0003
	FormatALaw       uint16 = 0x0006
	FormatMuLaw      uint16 = 0x0007
	FormatExtensible uint16 = 0xFFFE
)

// GUID is the 16-byte codec/subformat GUID used by WAVEFORMATEXTENSIBLE.
type GUID struct {
	Data1 uint32  `json:"data1"`
	Data2 uint16  `json:"data2"`
	Data3 uint16  `json:"data3"`
	Data4 [8]byte `json:"data4"`
}

func (g GUID) String() string {
	return fmt.Sprintf("%08x-%04x-%04x-%02x%02x-%02x%02x%02x%02x%02x%02x",
		g.Data1, g.Data2, g.Data3,
		g.Data4[0], g.Data4[1], g.Data4[2], g.Data4[3],
		g.Data4[4], g.Data4[5], g.Data4[6], g.Data4[7])
}

var (
	GUIDPCM        = GUID{0x00000001, 0x0000, 0x0010, [8]byte{0x80, 0x00, 0x00, 0xaa, 0x00, 0x38, 0x9b, 0x71}}
	GUIDFloat      = GUID{0x00000003, 0x0000, 0x0010, [8]byte{0x80, 0x00, 0x00, 0xaa, 0x00, 0x38, 0x9b, 0x71}}
	GUIDALaw       = GUID{0x00000006, 0x0000, 0x0010, [8]byte{0x80, 0x00, 0x00, 0xaa, 0x00, 0x38, 0x9b, 0x71}}
	GUIDMuLaw      = GUID{0x00000007, 0x0000, 0x0010, [8]byte{0x80, 0x00, 0x00, 0xaa, 0x00, 0x38, 0x9b, 0x71}}
	GUIDExtensible = GUID{}
)

// Format is the decoded fmt chunk plus codec declaration diagnostics.
type Format struct {
	FormatTag          uint16 `json:"format_tag"`
	FormatTagName      string `json:"format_tag_name"`
	Channels           int    `json:"channels"`
	SampleRate         int    `json:"sample_rate"`
	AvgBytesPerSecond  uint32 `json:"avg_bytes_per_second"`
	BlockAlign         int    `json:"block_align"`
	BitsPerSample      int    `json:"bits_per_sample"`
	ExtensionSize      int    `json:"extension_size,omitempty"`
	ValidBitsPerSample int    `json:"valid_bits_per_sample,omitempty"`
	ChannelMask        uint32 `json:"channel_mask,omitempty"`
	HasChannelMask     bool   `json:"has_channel_mask"`
	SubFormat          *GUID  `json:"sub_format,omitempty"`
	Codec              Codec  `json:"codec"`
}

type Codec struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Decodable   bool   `json:"decodable"`
	SampleKind  string `json:"sample_kind"`
}

type ChunkInfo struct {
	ID   string `json:"id"`
	Size int    `json:"size"`
}

// File contains parsed WAV metadata and deinterleaved normalized samples.
type File struct {
	Format    Format      `json:"format"`
	Chunks    []ChunkInfo `json:"chunks"`
	DataBytes []byte      `json:"-"`
	Frames    int         `json:"frames"`
	Samples   [][]float64 `json:"-"`
	Warnings  []string    `json:"warnings,omitempty"`
	RiffSize  uint32      `json:"riff_size"`
}

type rawChunk struct {
	id   [4]byte
	data []byte
}

// Parse reads a complete RIFF/WAVE stream. It uses only the Go standard library.
func Parse(r io.Reader) (*File, error) {
	input, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if len(input) < 12 {
		return nil, errors.New("wav: truncated RIFF/WAVE header")
	}
	if string(input[0:4]) != "RIFF" {
		return nil, errors.New("wav: missing RIFF magic")
	}
	if string(input[8:12]) != "WAVE" {
		return nil, errors.New("wav: RIFF form is not WAVE")
	}
	riffSize := binary.LittleEndian.Uint32(input[4:8])
	if int(riffSize)+8 != len(input) {
		return nil, fmt.Errorf("wav: RIFF size %d does not match input length %d", riffSize, len(input))
	}

	raw, infos, err := readChunks(input[12:])
	if err != nil {
		return nil, err
	}
	f := &File{RiffSize: riffSize, Chunks: infos}
	fmtIndex, dataIndex := -1, -1
	for i, ch := range raw {
		switch string(ch.id[:]) {
		case "fmt ":
			if fmtIndex != -1 {
				return nil, errors.New("wav: multiple fmt chunks")
			}
			fmtIndex = i
		case "data":
			if dataIndex != -1 {
				return nil, errors.New("wav: multiple data chunks")
			}
			dataIndex = i
		}
	}
	if fmtIndex < 0 {
		return nil, errors.New("wav: missing fmt chunk")
	}
	if dataIndex < 0 {
		return nil, errors.New("wav: missing data chunk")
	}
	if fmtIndex > dataIndex {
		return nil, errors.New("wav: fmt chunk must precede data chunk")
	}

	f.Format, err = parseFormat(raw[fmtIndex].data)
	if err != nil {
		return nil, err
	}
	f.DataBytes = raw[dataIndex].data
	if err := f.decode(); err != nil {
		return nil, err
	}
	return f, nil
}

func readChunks(body []byte) ([]rawChunk, []ChunkInfo, error) {
	var chunks []rawChunk
	var infos []ChunkInfo
	for off := 0; off < len(body); {
		if len(body)-off < 8 {
			return nil, nil, errors.New("wav: truncated chunk header")
		}
		var id [4]byte
		copy(id[:], body[off:off+4])
		size := int(binary.LittleEndian.Uint32(body[off+4 : off+8]))
		start, end := off+8, off+8+size
		if end > len(body) {
			return nil, nil, fmt.Errorf("wav: chunk %q size %d exceeds file", string(id[:]), size)
		}
		chunks = append(chunks, rawChunk{id: id, data: body[start:end]})
		infos = append(infos, ChunkInfo{ID: string(id[:]), Size: size})
		off = end
		if size%2 == 1 {
			if off >= len(body) {
				return nil, nil, fmt.Errorf("wav: chunk %q missing pad byte", string(id[:]))
			}
			off++
		}
	}
	return chunks, infos, nil
}

func parseFormat(data []byte) (Format, error) {
	if len(data) < 16 {
		return Format{}, errors.New("wav: fmt chunk is shorter than 16 bytes")
	}
	out := Format{
		FormatTag:         binary.LittleEndian.Uint16(data[0:2]),
		Channels:          int(binary.LittleEndian.Uint16(data[2:4])),
		SampleRate:        int(binary.LittleEndian.Uint32(data[4:8])),
		AvgBytesPerSecond: binary.LittleEndian.Uint32(data[8:12]),
		BlockAlign:        int(binary.LittleEndian.Uint16(data[12:14])),
		BitsPerSample:     int(binary.LittleEndian.Uint16(data[14:16])),
	}
	if out.Channels <= 0 {
		return out, errors.New("wav: nChannels must be positive")
	}
	if out.SampleRate <= 0 {
		return out, errors.New("wav: nSamplesPerSec must be positive")
	}
	if len(data) >= 18 {
		out.ExtensionSize = int(binary.LittleEndian.Uint16(data[16:18]))
		if out.ExtensionSize != len(data)-18 {
			return out, fmt.Errorf("wav: cbSize %d does not match fmt extension %d bytes", out.ExtensionSize, len(data)-18)
		}
	}
	if out.FormatTag == FormatExtensible {
		if len(data) < 40 || out.ExtensionSize < 22 {
			return out, errors.New("wav: WAVE_FORMAT_EXTENSIBLE requires a 40-byte fmt chunk")
		}
		out.ValidBitsPerSample = int(binary.LittleEndian.Uint16(data[18:20]))
		out.ChannelMask = binary.LittleEndian.Uint32(data[20:24])
		out.HasChannelMask = true
		g := GUID{
			Data1: binary.LittleEndian.Uint32(data[24:28]),
			Data2: binary.LittleEndian.Uint16(data[28:30]),
			Data3: binary.LittleEndian.Uint16(data[30:32]),
		}
		copy(g.Data4[:], data[32:40])
		out.SubFormat = &g
	}
	out.Codec = codecFor(out)
	out.FormatTagName = out.Codec.Description
	return out, nil
}

func codecFor(f Format) Codec {
	tag := f.FormatTag
	id, desc, kind, decodable := "", "", "unsupported", false
	if tag == FormatExtensible && f.SubFormat != nil {
		g := *f.SubFormat
		switch g {
		case GUIDPCM:
			tag = FormatPCM
		case GUIDFloat:
			tag = FormatIEEEFloat
		case GUIDALaw:
			tag = FormatALaw
		case GUIDMuLaw:
			tag = FormatMuLaw
		default:
			return Codec{ID: "extensible:" + g.String(), Description: "WAVE_FORMAT_EXTENSIBLE compressed/unknown subformat", Decodable: false, SampleKind: "unknown"}
		}
	}
	switch tag {
	case FormatPCM:
		id = fmt.Sprintf("pcm_s%dle", f.BitsPerSample)
		desc = "little-endian integer PCM"
		kind, decodable = "pcm-integer", f.BitsPerSample == 8 || f.BitsPerSample == 16 || f.BitsPerSample == 24 || f.BitsPerSample == 32
	case FormatIEEEFloat:
		id = fmt.Sprintf("float%dle", f.BitsPerSample)
		desc = "little-endian IEEE float PCM"
		kind, decodable = "ieee-float", f.BitsPerSample == 32 || f.BitsPerSample == 64
	case FormatALaw:
		id, desc, kind = "g711a", "G.711 A-law", "g711"
	case FormatMuLaw:
		id, desc, kind = "g711u", "G.711 mu-law", "g711"
	default:
		id = fmt.Sprintf("format_tag_0x%04x", f.FormatTag)
		desc = "unsupported or compressed WAV format"
	}
	return Codec{ID: id, Description: desc, Decodable: decodable, SampleKind: kind}
}

func (f *File) decode() error {
	format := f.Format
	if !format.Codec.Decodable {
		f.Warnings = append(f.Warnings, "codec samples cannot be decoded without a codec implementation; metadata-only diagnosis remains possible")
		return nil
	}
	bytesPerSample := format.BitsPerSample / 8
	if format.BitsPerSample%8 != 0 || bytesPerSample <= 0 {
		return fmt.Errorf("wav: unsupported bits per sample %d", format.BitsPerSample)
	}
	frameBytes := format.Channels * bytesPerSample
	if format.BlockAlign != 0 && format.BlockAlign != frameBytes {
		return fmt.Errorf("wav: nBlockAlign %d does not match %d channels × %d bytes", format.BlockAlign, format.Channels, bytesPerSample)
	}
	if len(f.DataBytes)%frameBytes != 0 {
		return fmt.Errorf("wav: data length %d is not an integral number of %d-byte frames", len(f.DataBytes), frameBytes)
	}
	f.Frames = len(f.DataBytes) / frameBytes
	f.Samples = make([][]float64, format.Channels)
	for ch := range f.Samples {
		f.Samples[ch] = make([]float64, f.Frames)
	}
	for frame := 0; frame < f.Frames; frame++ {
		base := frame * frameBytes
		for ch := 0; ch < format.Channels; ch++ {
			start := base + ch*bytesPerSample
			v, err := decodeSample(f.DataBytes[start:start+bytesPerSample], format.BitsPerSample, format.Codec.SampleKind)
			if err != nil {
				return err
			}
			f.Samples[ch][frame] = v
		}
	}
	return nil
}

func decodeSample(b []byte, bits int, kind string) (float64, error) {
	switch kind {
	case "ieee-float":
		if bits == 32 {
			return float64(math.Float32frombits(binary.LittleEndian.Uint32(b))), nil
		}
		return math.Float64frombits(binary.LittleEndian.Uint64(b)), nil
	case "pcm-integer":
		switch bits {
		case 8:
			return (float64(b[0]) - 128) / 128.0, nil
		case 16:
			return float64(int16(binary.LittleEndian.Uint16(b))) / 32768.0, nil
		case 24:
			x := int32(b[0]) | int32(b[1])<<8 | int32(b[2])<<16
			if x&0x800000 != 0 {
				x |= ^0xffffff
			}
			return float64(x) / 8388608.0, nil
		case 32:
			return float64(int32(binary.LittleEndian.Uint32(b))) / 2147483648.0, nil
		}
	}
	return 0, fmt.Errorf("wav: no decoder for %s/%d", kind, bits)
}
