package mka

import (
	"fmt"
	"sort"
)

type OutputSample struct {
	TrackID        uint64
	PacketSequence int
	Sample         int64
	Preroll        bool
}

type Decoder struct {
	track Track
}

func NewDecoder(track Track) Decoder {
	return Decoder{track: track}
}

func (decoder Decoder) findPacket(decodeStart int64) (Packet, error) {
	index := sort.Search(len(decoder.track.Packets), func(index int) bool {
		return decoder.track.Packets[index].DecodeStartSample >= decodeStart
	})
	if index == len(decoder.track.Packets) {
		index = len(decoder.track.Packets) - 1
	} else if decoder.track.Packets[index].DecodeStartSample > decodeStart {
		index--
	}
	if index < 0 || decoder.track.Packets[index].DecodeStartSample > decodeStart {
		return Packet{}, fmt.Errorf("track %d has no packet at or before decode sample %d", decoder.track.Spec.ID, decodeStart)
	}
	return decoder.track.Packets[index], nil
}

func (decoder Decoder) packetRange(first Packet) []Packet {
	packets := make([]Packet, 0)
	for _, packet := range decoder.track.Packets {
		if packet.Sequence >= first.Sequence {
			packets = append(packets, packet)
		}
	}
	return packets
}

func (decoder Decoder) decode(packets []Packet, validFrom int64) []OutputSample {
	outputs := make([]OutputSample, 0)
	for _, packet := range packets {
		for offset := int64(0); offset < packet.DurationSamples; offset++ {
			sample := packet.DecodeStartSample + offset
			outputs = append(outputs, OutputSample{
				TrackID:        decoder.track.Spec.ID,
				PacketSequence: packet.Sequence,
				Sample:         sample,
				Preroll:        sample < validFrom,
			})
		}
	}
	return outputs
}

func (decoder Decoder) DecodeFromStart() []OutputSample {
	if len(decoder.track.Packets) == 0 {
		return nil
	}
	first := decoder.track.Packets[0]
	return decoder.decode(decoder.packetRange(first), decoder.track.Spec.FirstOutputSample)
}

func (decoder Decoder) SeekTo(targetSample int64) ([]OutputSample, Packet, error) {
	fetchStart := targetSample - decoder.track.Spec.SeekPreRollSamples
	firstDecodeStart := decoder.track.Packets[0].DecodeStartSample
	if fetchStart < firstDecodeStart {
		fetchStart = firstDecodeStart
	}

	firstPacket, err := decoder.findPacket(fetchStart)
	if err != nil {
		return nil, Packet{}, err
	}
	return decoder.decode(decoder.packetRange(firstPacket), targetSample), firstPacket, nil
}

func VisibleSamples(samples []OutputSample) []OutputSample {
	visible := make([]OutputSample, 0, len(samples))
	for _, sample := range samples {
		if !sample.Preroll {
			visible = append(visible, sample)
		}
	}
	return visible
}
