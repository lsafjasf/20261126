package mka

import "testing"

func assertSeek(t *testing.T, track Track, target, expectedPacket int64) {
	t.Helper()
	decoder := NewDecoder(track)
	samples, firstPacket, err := decoder.SeekTo(target)
	if err != nil {
		t.Fatalf("seek to sample %d failed: %v", target, err)
	}
	if got, want := int64(firstPacket.Sequence), expectedPacket; got != want {
		t.Fatalf("seek target %d starts packet %d, want %d", target, got, want)
	}
	visible := VisibleSamples(samples)
	if len(visible) == 0 {
		t.Fatalf("seek target %d produced no visible samples", target)
	}
	if got, want := visible[0].Sample, target; got != want {
		t.Fatalf("first visible sample after seek = %d, want %d", got, want)
	}

	repeated, repeatedPacket, err := decoder.SeekTo(target)
	if err != nil {
		t.Fatalf("repeated seek to sample %d failed: %v", target, err)
	}
	if repeatedPacket.Sequence != firstPacket.Sequence || len(repeated) != len(samples) {
		t.Fatalf("repeated seek to %d changed decoder state", target)
	}
	if VisibleSamples(repeated)[0].Sample != target {
		t.Fatalf("repeated seek changed first visible sample for target %d", target)
	}
}

func TestRepeatedSeeksUsePerTrackPreroll(t *testing.T) {
	tracks := testTracks()
	opus, aac := tracks[0], tracks[1]

	opusTargets := []int64{19_200, 4_800, 28_800, 19_200}
	opusPackets := []int64{13, 0, 23, 13}
	for index, target := range opusTargets {
		assertSeek(t, opus, target, opusPackets[index])
	}

	aacTargets := []int64{11_264, 3_072, 25_600, 11_264}
	aacPackets := []int64{11, 3, 25, 11}
	for index, target := range aacTargets {
		assertSeek(t, aac, target, aacPackets[index])
	}
}

func TestSeekPrerollIsDiscardedNotShifted(t *testing.T) {
	tracks := testTracks()
	opus := tracks[0]

	target := int64(19_200)
	samples, firstPacket, err := NewDecoder(opus).SeekTo(target)
	if err != nil {
		t.Fatal(err)
	}
	fetchPoint := target - opus.Spec.SeekPreRollSamples
	wantPacket := int64(13)
	if firstPacket.Sequence != int(wantPacket) {
		t.Fatalf("fetch packet = %d, want %d", firstPacket.Sequence, wantPacket)
	}
	if firstPacket.DecodeStartSample > fetchPoint {
		t.Fatalf("decode fetch point %d is after requested fetch point %d", firstPacket.DecodeStartSample, fetchPoint)
	}
	if got, want := firstPacket.DecodeStartSample, int64(14_568); got != want {
		t.Fatalf("decode fetch point = %d, want %d", got, want)
	}
	if got, want := len(samples), int(opus.Spec.FrameSamples)*27; got != want {
		t.Fatalf("decoded sample count = %d, want %d", got, want)
	}
	if got, want := len(VisibleSamples(samples)), 21_288; got != want {
		t.Fatalf("visible sample count = %d, want %d", got, want)
	}
	if samples[0].Sample >= target || !samples[0].Preroll {
		t.Fatalf("sample %+v must be discarded seek preroll", samples[0])
	}
}
