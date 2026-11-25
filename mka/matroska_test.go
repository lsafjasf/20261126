package mka

import "testing"

func TestMatroskaStoresRawTimestampsAndPerTrackDelays(t *testing.T) {
	tracks := testTracks()
	document := SerializeMatroska(tracks)
	file, err := ParseMatroska(document)
	if err != nil {
		t.Fatal(err)
	}

	if len(file.Tracks) != 2 {
		t.Fatalf("stored track count = %d, want 2", len(file.Tracks))
	}
	for index, track := range file.Tracks {
		if got, want := track.Type, uint64(2); got != want {
			t.Fatalf("track %d type = %d, want audio %d", index, got, want)
		}
	}
	if got, want := file.Tracks[0].CodecDelayNS, uint64(6_500_000); got != want {
		t.Fatalf("stored Opus CodecDelay = %d, want %d", got, want)
	}
	if got, want := file.Tracks[0].SeekPreRollNS, uint64(80_000_000); got != want {
		t.Fatalf("stored Opus SeekPreRoll = %d, want %d", got, want)
	}
	if got, want := file.Tracks[1].CodecDelayNS, uint64(47_891_156); got != want {
		t.Fatalf("stored AAC CodecDelay = %d, want %d", got, want)
	}
	if got, want := file.Tracks[1].SeekPreRollNS, uint64(23_219_954); got != want {
		t.Fatalf("stored AAC SeekPreRoll = %d, want %d", got, want)
	}

	firstBlocks := map[uint64]StoredBlock{}
	for _, block := range file.Blocks {
		if _, exists := firstBlocks[block.TrackID]; !exists {
			firstBlocks[block.TrackID] = block
		}
	}

	opusBlock := firstBlocks[1]
	if got, want := opusBlock.TimestampNS, tracks[0].Packets[0].RawTimestampNS; got != want {
		t.Fatalf("stored Opus first block = %d, want raw %d", got, want)
	}
	aacBlock := firstBlocks[2]
	if got, want := aacBlock.TimestampNS, tracks[1].Packets[0].RawTimestampNS; got != want {
		t.Fatalf("stored AAC first block = %d, want raw %d", got, want)
	}
	if aacBlock.TimestampNS%1_000_000 == 0 {
		t.Fatalf("AAC timestamp %d is accidentally millisecond aligned", aacBlock.TimestampNS)
	}
}

func TestStoredBlockRoundTripMatchesPackets(t *testing.T) {
	tracks := testTracks()
	file, err := ParseMatroska(SerializeMatroska(tracks))
	if err != nil {
		t.Fatal(err)
	}

	for _, track := range tracks {
		blockIndex := 0
		for _, block := range file.Blocks {
			if block.TrackID != track.Spec.ID {
				continue
			}
			packet := track.Packets[blockIndex]
			if block.TimestampNS != packet.RawTimestampNS {
				t.Fatalf("track %d block %d timestamp = %d, want %d", track.Spec.ID, blockIndex, block.TimestampNS, packet.RawTimestampNS)
			}
			if string(block.Payload) != string(packet.Payload) {
				t.Fatalf("track %d block %d payload changed", track.Spec.ID, blockIndex)
			}
			blockIndex++
		}
		if blockIndex != len(track.Packets) {
			t.Fatalf("track %d stored %d blocks, want %d", track.Spec.ID, blockIndex, len(track.Packets))
		}
	}
}
