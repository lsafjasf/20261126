package main

import (
	"fmt"

	"uid4-0021/mka-timing/mka"
)

func demoTrack(spec mka.TrackSpec, seekTargets []int64) mka.Track {
	track := mka.BuildTrack(spec)
	first := track.Packets[0]

	fmt.Printf("%s track %d\n", spec.Codec, spec.ID)
	fmt.Printf("  first packet raw timestamp: %d ns\n", first.RawTimestampNS)
	fmt.Printf("  CodecDelay: %d samples => %d ns\n", spec.CodecDelaySamples, track.CodecDelayNS)
	fmt.Printf("  actual first frame: %d ns\n", mka.ActualFrameTimestamp(first, track))
	fmt.Printf("  SeekPreRoll: %d samples => %d ns\n", spec.SeekPreRollSamples, track.SeekPreRollNS)

	decoder := mka.NewDecoder(track)
	for _, target := range seekTargets {
		_, packet, err := decoder.SeekTo(target)
		if err != nil {
			panic(err)
		}
		fmt.Printf("  seek %5d -> fetch packet %2d at decode sample %d\n", target, packet.Sequence, packet.DecodeStartSample)
	}
	fmt.Println()
	return track
}

func main() {
	opus := demoTrack(mka.TrackSpec{
		ID: 1, Codec: mka.CodecOpus, SampleRate: 48_000,
		FrameSamples: 960, FirstOutputSample: 2_400,
		CodecDelaySamples: 312, SeekPreRollSamples: 3_840, FrameCount: 40,
	}, []int64{19_200, 4_800, 28_800, 19_200})

	aac := demoTrack(mka.TrackSpec{
		ID: 2, Codec: mka.CodecAAC, SampleRate: 44_100,
		FrameSamples: 1_024, FirstOutputSample: 705,
		CodecDelaySamples: 2_112, SeekPreRollSamples: 1_024, FrameCount: 40,
	}, []int64{11_264, 3_072, 25_600, 11_264})

	file, err := mka.ParseMatroska(mka.SerializeMatroska([]mka.Track{opus, aac}))
	if err != nil {
		panic(err)
	}
	fmt.Printf("serialized %d tracks and %d SimpleBlocks\n", len(file.Tracks), len(file.Blocks))
}
