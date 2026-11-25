package opusalign

import (
	"encoding/binary"
	"fmt"
)

const opusHeadMagic = "OpusHead"

type OpusHead struct {
	Version         byte   `json:"version"`
	Channels        int    `json:"channels"`
	PreSkip         uint16 `json:"pre_skip"`
	InputSampleRate uint32 `json:"input_sample_rate"`
	OutputGain      int16  `json:"output_gain"`
	MappingFamily   byte   `json:"mapping_family"`
}

func ParseOpusHead(data []byte) (OpusHead, error) {
	if len(data) < 19 {
		return OpusHead{}, fmt.Errorf("OpusHead is %d bytes, need at least 19", len(data))
	}
	if string(data[:8]) != opusHeadMagic {
		return OpusHead{}, fmt.Errorf("bad OpusHead magic %q", data[:8])
	}

	head := OpusHead{
		Version:         data[8],
		Channels:        int(data[9]),
		PreSkip:         binary.LittleEndian.Uint16(data[10:12]),
		InputSampleRate: binary.LittleEndian.Uint32(data[12:16]),
		OutputGain:      int16(binary.LittleEndian.Uint16(data[16:18])),
		MappingFamily:   data[18],
	}
	if head.Channels < 1 {
		return OpusHead{}, fmt.Errorf("OpusHead channel count is %d", head.Channels)
	}
	if head.MappingFamily != 0 && len(data) < 21+head.Channels {
		return OpusHead{}, fmt.Errorf("truncated channel mapping table")
	}
	return head, nil
}

func PreSkipFromOpusHead(data []byte) (int, error) {
	head, err := ParseOpusHead(data)
	if err != nil {
		return 0, err
	}
	return int(head.PreSkip), nil
}
