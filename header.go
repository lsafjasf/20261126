package adts

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	// FrameHeaderSize is the size of an ADTS header without a CRC.
	FrameHeaderSize = 7

	// FrameHeaderCRCSize is the additional size when protection_absent is 0.
	FrameHeaderCRCSize = 9

	// SamplesPerRawDataBlock is the number of PCM samples represented by one
	// AAC raw data block in an ADTS frame.
	SamplesPerRawDataBlock = 1024
)

var (
	ErrShortHeader     = errors.New("adts: short frame header")
	ErrSyncWord        = errors.New("adts: missing syncword")
	ErrLayer           = errors.New("adts: unsupported layer")
	ErrSampleRateIndex = errors.New("adts: invalid sampling frequency index")
	ErrFrameLength     = errors.New("adts: invalid frame length")
)

var sampleRates = [...]int{
	96000, 88200, 64000, 48000, 44100, 32000, 24000,
	22050, 16000, 12000, 11025, 8000, 7350,
}

// State is the audio configuration that must stay unchanged within one
// segment. A change at a valid frame boundary starts a new segment.
type State struct {
	MPEGVersion     int `json:"mpegVersion"`
	Profile         int `json:"profile"`
	SampleRateIndex int `json:"sampleRateIndex"`
	SampleRate      int `json:"sampleRate"`
	ChannelConfig   int `json:"channelConfiguration"`
}

// Header contains the fields of an ADTS frame header that are needed for
// framing and PCM sample accounting.
type Header struct {
	State
	ProtectionAbsent bool `json:"protectionAbsent"`
	FrameLength      int  `json:"frameLength"`
	HeaderLength     int  `json:"headerLength"`
	RawDataBlocks    int  `json:"rawDataBlocks"`
}

// Samples returns the number of PCM samples carried by the frame.
func (h Header) Samples() int64 {
	return int64(SamplesPerRawDataBlock * (h.RawDataBlocks + 1))
}

// ParseHeader parses the fixed ADTS header at the beginning of b. It does not
// require the complete frame to be present.
func ParseHeader(b []byte) (Header, error) {
	if len(b) < FrameHeaderSize {
		return Header{}, ErrShortHeader
	}
	if b[0] != 0xff || b[1]&0xf0 != 0xf0 {
		return Header{}, ErrSyncWord
	}

	mpegID := int((b[1] >> 3) & 0x01)
	layer := int((b[1] >> 1) & 0x03)
	if layer != 0 {
		return Header{}, fmt.Errorf("%w: layer %d", ErrLayer, layer)
	}

	profile := int(b[2] >> 6)
	sampleRateIndex := int((b[2] >> 2) & 0x0f)
	if sampleRateIndex >= len(sampleRates) {
		return Header{}, fmt.Errorf("%w: %d", ErrSampleRateIndex, sampleRateIndex)
	}

	channelConfig := int(b[2]&0x01)<<2 | int(b[3]>>6)
	frameLength := int(b[3]&0x03)<<11 | int(b[4])<<3 | int(b[5]>>5)
	protectionAbsent := b[1]&0x01 != 0
	headerLength := FrameHeaderSize
	if !protectionAbsent {
		headerLength = FrameHeaderCRCSize
	}
	if frameLength < headerLength {
		return Header{}, fmt.Errorf(
			"%w: %d is smaller than header size %d",
			ErrFrameLength, frameLength, headerLength,
		)
	}
	if len(b) < headerLength {
		return Header{}, ErrShortHeader
	}

	mpegVersion := 4
	if mpegID == 1 {
		mpegVersion = 2
	}

	return Header{
		State: State{
			MPEGVersion:     mpegVersion,
			Profile:         profile,
			SampleRateIndex: sampleRateIndex,
			SampleRate:      sampleRates[sampleRateIndex],
			ChannelConfig:   channelConfig,
		},
		ProtectionAbsent: protectionAbsent,
		FrameLength:      frameLength,
		HeaderLength:     headerLength,
		RawDataBlocks:    int(b[6] & 0x03),
	}, nil
}

// SyncWordAt reports whether b starts with an ADTS syncword.
func SyncWordAt(b []byte) bool {
	return len(b) >= 2 && binary.BigEndian.Uint16(b[:2])&0xfff0 == 0xfff0
}
