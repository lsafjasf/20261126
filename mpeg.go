package mp3timeline

import (
	"errors"
	"fmt"
)

// ErrNoMPEGFrames is returned when no complete MPEG audio frame is found.
var ErrNoMPEGFrames = errors.New("mp3timeline: no MPEG audio frames found")

type mpegVersion uint8

const (
	mpeg25 mpegVersion = iota
	mpeg2
	mpeg1
)

func (v mpegVersion) String() string {
	switch v {
	case mpeg1:
		return "1"
	case mpeg2:
		return "2"
	case mpeg25:
		return "2.5"
	default:
		return "unknown"
	}
}

type mpegHeader struct {
	version         mpegVersion
	layer           int
	sampleRate      int
	bitrate         int
	samplesPerFrame int
	frameSize       int
	channels        int
	crc             bool
	padding         bool
	sideInfoSize    int
}

func (h mpegHeader) compatible(other mpegHeader) bool {
	return h.version == other.version &&
		h.layer == other.layer &&
		h.sampleRate == other.sampleRate
}

func (h mpegHeader) xingOffset() int {
	offset := 4 + h.sideInfoSize
	if h.crc {
		offset += 2
	}
	return offset
}

var bitrateTable = map[mpegVersion][3][16]int{
	mpeg1: {
		{0, 32, 64, 96, 128, 160, 192, 224, 256, 288, 320, 352, 384, 416, 448, 0},
		{0, 32, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 384, 0},
		{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 0},
	},
	mpeg2: {
		{0, 32, 48, 56, 64, 80, 96, 112, 128, 144, 160, 176, 192, 224, 256, 0},
		{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, 0},
		{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, 0},
	},
	// MPEG 2.5 uses the MPEG 2 bitrate tables.
	mpeg25: {
		{0, 32, 48, 56, 64, 80, 96, 112, 128, 144, 160, 176, 192, 224, 256, 0},
		{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, 0},
		{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, 0},
	},
}

var sampleRateTable = map[mpegVersion][3]int{
	mpeg1:  {44100, 48000, 32000},
	mpeg2:  {22050, 24000, 16000},
	mpeg25: {11025, 12000, 8000},
}

func parseMPEGHeader(b []byte) (mpegHeader, error) {
	if len(b) < 4 {
		return mpegHeader{}, fmt.Errorf("mp3timeline: short MPEG header")
	}
	raw := uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
	if raw>>21 != 0x7ff {
		return mpegHeader{}, fmt.Errorf("mp3timeline: MPEG sync not found")
	}

	var version mpegVersion
	switch (raw >> 19) & 3 {
	case 0:
		version = mpeg25
	case 2:
		version = mpeg2
	case 3:
		version = mpeg1
	default:
		return mpegHeader{}, fmt.Errorf("mp3timeline: reserved MPEG version")
	}

	layerBits := (raw >> 17) & 3
	if layerBits == 0 {
		return mpegHeader{}, fmt.Errorf("mp3timeline: reserved MPEG layer")
	}
	layer := int(4 - layerBits)
	sampleRateIndex := (raw >> 10) & 3
	if sampleRateIndex == 3 {
		return mpegHeader{}, fmt.Errorf("mp3timeline: reserved sample rate")
	}
	bitrateIndex := (raw >> 12) & 15
	if bitrateIndex == 0 || bitrateIndex == 15 {
		return mpegHeader{}, fmt.Errorf("mp3timeline: free/invalid bitrate")
	}

	bitrate := bitrateTable[version][layer-1][bitrateIndex] * 1000
	if bitrate == 0 {
		return mpegHeader{}, fmt.Errorf("mp3timeline: invalid bitrate index")
	}
	sampleRate := sampleRateTable[version][sampleRateIndex]
	padding := (raw>>9)&1 == 1
	crc := (raw>>16)&1 == 0
	channelMode := (raw >> 6) & 3

	spf := samplesPerFrame(version, layer)
	frameSize := frameLength(layer, spf, bitrate, sampleRate, padding)
	if frameSize < 4 {
		return mpegHeader{}, fmt.Errorf("mp3timeline: invalid frame length %d", frameSize)
	}

	return mpegHeader{
		version:         version,
		layer:           layer,
		sampleRate:      sampleRate,
		bitrate:         bitrate,
		samplesPerFrame: spf,
		frameSize:       frameSize,
		channels:        channels(channelMode),
		crc:             crc,
		padding:         padding,
		sideInfoSize:    sideInfoSize(version, layer, channelMode),
	}, nil
}

func samplesPerFrame(version mpegVersion, layer int) int {
	if layer == 1 {
		return 384
	}
	if layer == 3 && version != mpeg1 {
		return 576
	}
	return 1152
}

func frameLength(layer, spf, bitrate, sampleRate int, padding bool) int {
	if layer == 1 {
		n := (12*bitrate)/sampleRate + boolInt(padding)
		return n * 4
	}
	n := spf * bitrate / (8 * sampleRate)
	if padding {
		n++
	}
	return n
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func channels(mode uint32) int {
	if mode == 3 {
		return 1
	}
	return 2
}

func sideInfoSize(version mpegVersion, layer int, mode uint32) int {
	if layer != 3 {
		return 0
	}
	if version == mpeg1 {
		if mode == 3 {
			return 17
		}
		return 32
	}
	if mode == 3 {
		return 9
	}
	return 17
}
