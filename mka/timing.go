package mka

import "math/big"

const nanosPerSecond int64 = 1_000_000_000

type Codec string

const (
	CodecOpus Codec = "A_OPUS"
	CodecAAC  Codec = "A_AAC"
)

type TrackSpec struct {
	ID                 uint64
	Codec              Codec
	SampleRate         int64
	FrameSamples       int64
	FirstOutputSample  int64
	CodecDelaySamples  int64
	SeekPreRollSamples int64
	FrameCount         int
}

type Packet struct {
	TrackID            uint64
	Sequence           int
	DecodeStartSample  int64
	RawTimestampSample int64
	RawTimestampNS     int64
	DurationSamples    int64
	DurationNS         int64
	Payload            []byte
}

type Track struct {
	Spec          TrackSpec
	CodecDelayNS  int64
	SeekPreRollNS int64
	Packets       []Packet
}

func floorDiv(numerator, denominator int64) int64 {
	quotient := numerator / denominator
	if (numerator^denominator) < 0 && numerator%denominator != 0 {
		quotient--
	}
	return quotient
}

func samplesToNS(samples, sampleRate int64) int64 {
	return floorDiv(samples*nanosPerSecond, sampleRate)
}

func sampleTimeRational(samples, sampleRate int64) *big.Rat {
	return big.NewRat(samples*nanosPerSecond, sampleRate)
}

func BuildTrack(spec TrackSpec) Track {
	if spec.SampleRate <= 0 || spec.FrameSamples <= 0 || spec.FrameCount <= 0 {
		panic("invalid track specification")
	}
	if spec.FirstOutputSample < 0 || spec.CodecDelaySamples < 0 || spec.SeekPreRollSamples < 0 {
		panic("sample positions and discard counts cannot be negative")
	}

	track := Track{
		Spec:          spec,
		CodecDelayNS:  samplesToNS(spec.CodecDelaySamples, spec.SampleRate),
		SeekPreRollNS: samplesToNS(spec.SeekPreRollSamples, spec.SampleRate),
	}

	for sequence := 0; sequence < spec.FrameCount; sequence++ {
		index := int64(sequence)
		rawSample := spec.FirstOutputSample + index*spec.FrameSamples
		track.Packets = append(track.Packets, Packet{
			TrackID:            spec.ID,
			Sequence:           sequence,
			DecodeStartSample:  rawSample - spec.CodecDelaySamples,
			RawTimestampSample: rawSample,
			RawTimestampNS:     samplesToNS(rawSample, spec.SampleRate),
			DurationSamples:    spec.FrameSamples,
			DurationNS:         samplesToNS(spec.FrameSamples, spec.SampleRate),
			Payload: []byte{
				byte(sequence >> 8),
				byte(sequence),
			},
		})
	}

	return track
}

func ActualFrameTimestamp(packet Packet, track Track) int64 {
	return packet.RawTimestampNS - track.CodecDelayNS
}

func ActualFrameTimestampRational(packet Packet, track Track) *big.Rat {
	sample := packet.RawTimestampSample - track.Spec.CodecDelaySamples
	return sampleTimeRational(sample, track.Spec.SampleRate)
}
