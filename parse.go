package eac3

import (
	"errors"
	"fmt"
)

const SyncWord uint16 = 0x0B77

var (
	ErrShortHeader = errors.New("eac3: short frame header")
	ErrSyncWord    = errors.New("eac3: invalid sync word")
	ErrFrameType   = errors.New("eac3: reserved strmtyp")
	ErrSampleRate  = errors.New("eac3: invalid sample rate code")
)

type bitReader struct {
	buf []byte
	pos uint
}

func (r *bitReader) read(n uint) (uint32, error) {
	if n > 32 || r.pos+n > uint(len(r.buf))*8 {
		return 0, ErrShortHeader
	}
	var value uint32
	for i := uint(0); i < n; i++ {
		byteIndex := (r.pos + i) / 8
		bitIndex := 7 - ((r.pos + i) % 8)
		bit := (r.buf[byteIndex] >> bitIndex) & 1
		value = (value << 1) | uint32(bit)
	}
	r.pos += n
	return value, nil
}

// Header 包含解码前固定帧头字段。DRC 等条件元数据由调用方在进入状态图前
// 解析并通过 Enrich 填回；状态图不把缺失 DRC 误当成“DRC 未变化”。
type Header struct {
	FrameInfo
	BSID uint8 `json:"bsid,omitempty"`
}

// ParseHeader 解析 syncword 之后的最小 E-AC-3 syncinfo/BSI 固定字段。
func ParseHeader(data []byte) (Header, error) {
	if len(data) < 5 {
		return Header{}, fmt.Errorf("%w: need 5 bytes, got %d", ErrShortHeader, len(data))
	}
	if uint16(data[0])<<8|uint16(data[1]) != SyncWord {
		return Header{}, ErrSyncWord
	}

	r := &bitReader{buf: data[:5]}
	if _, err := r.read(16); err != nil {
		return Header{}, err
	}
	frameType, _ := r.read(2)
	substreamID, _ := r.read(3)
	frameSizeCode, _ := r.read(11)
	fsCode, err := r.read(2)
	if err != nil {
		return Header{}, err
	}

	h := Header{}
	h.Type = SubstreamType(frameType)
	if h.Type == SubstreamReserved {
		return Header{}, ErrFrameType
	}
	h.SubstreamID = uint8(substreamID)
	h.Spec.Boundary.FrameSizeBytes = (int(frameSizeCode) + 1) << 1

	blocks := uint8(6)
	switch fsCode {
	case 0:
		h.Spec.SampleRateHz = 48000
	case 1:
		h.Spec.SampleRateHz = 44100
	case 2:
		h.Spec.SampleRateHz = 32000
	case 3:
		fsCode2, err := r.read(2)
		if err != nil {
			return Header{}, err
		}
		switch fsCode2 {
		case 0:
			h.Spec.SampleRateHz = 24000
		case 1:
			h.Spec.SampleRateHz = 22050
		case 2:
			h.Spec.SampleRateHz = 16000
		default:
			return Header{}, ErrSampleRate
		}
	default:
		return Header{}, ErrSampleRate
	}

	if fsCode != 3 {
		blockCode, err := r.read(2)
		if err != nil {
			return Header{}, err
		}
		blocks = [4]uint8{1, 2, 3, 6}[blockCode]
	}

	acmod, err := r.read(3)
	if err != nil {
		return Header{}, err
	}
	lfe, err := r.read(1)
	if err != nil {
		return Header{}, err
	}
	if len(data) >= 7 {
		full := &bitReader{buf: data[:7], pos: r.pos}
		if bsid, err := full.read(5); err == nil {
			h.BSID = uint8(bsid)
		}
	}

	h.Spec.Channel = ChannelStructure{
		Acmod:    uint8(acmod),
		LFEOn:    lfe == 1,
		Channels: acmodChannels[acmod] + boolToInt(lfe == 1),
		Labels:   append([]string(nil), acmodLabels[acmod]...),
	}
	h.Spec.Boundary.BlocksPerFrame = blocks
	h.Spec.Boundary.SamplesPerFrame = int(blocks) * 256
	return h, nil
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

var acmodChannels = [8]int{2, 1, 2, 3, 3, 4, 4, 5}

var acmodLabels = [8][]string{
	{"Ch1", "Ch2"},
	{"C"},
	{"L", "R"},
	{"L", "C", "R"},
	{"L", "R", "S"},
	{"L", "C", "R", "S"},
	{"L", "R", "SL", "SR"},
	{"L", "C", "R", "SL", "SR"},
}

// FindSync 在 data[from:] 中寻找下一个 E-AC-3 syncword。
func FindSync(data []byte, from int) int {
	if from < 0 {
		from = 0
	}
	for i := from; i+1 < len(data); i++ {
		if data[i] == 0x0B && data[i+1] == 0x77 {
			return i
		}
	}
	return -1
}

// ScanHeaders 按 frmsiz 推进；遇到坏帧时搜索下一个 syncword。
// 返回的 consumed 是成功扫描覆盖到的字节数，尾部不完整帧不会被吞掉。
func ScanHeaders(data []byte) (headers []Header, consumed int, err error) {
	pos := FindSync(data, 0)
	if pos < 0 {
		return nil, 0, ErrSyncWord
	}
	for pos+5 <= len(data) {
		h, perr := ParseHeader(data[pos:])
		if perr != nil {
			next := FindSync(data, pos+2)
			if next < 0 {
				return headers, consumed, perr
			}
			pos = next
			continue
		}
		h.ByteOffset = int64(pos)
		headers = append(headers, h)
		consumed = pos + h.Spec.Boundary.FrameSizeBytes
		next := pos + h.Spec.Boundary.FrameSizeBytes
		if next <= pos || next > len(data) {
			return headers, pos, nil
		}
		pos = next
	}
	return headers, consumed, nil
}
