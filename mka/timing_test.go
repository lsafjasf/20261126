package mka

import (
	"math/big"
	"testing"
)

const (
	opusRate = 48_000
	aacRate  = 44_100
)

func testTracks() []Track {
	opus := BuildTrack(TrackSpec{
		ID:                 1,
		Codec:              CodecOpus,
		SampleRate:         opusRate,
		FrameSamples:       960,
		FirstOutputSample:  2_400,
		CodecDelaySamples:  312,
		SeekPreRollSamples: 3_840,
		FrameCount:         40,
	})
	aac := BuildTrack(TrackSpec{
		ID:                 2,
		Codec:              CodecAAC,
		SampleRate:         aacRate,
		FrameSamples:       1_024,
		FirstOutputSample:  705,
		CodecDelaySamples:  2_112,
		SeekPreRollSamples: 1_024,
		FrameCount:         40,
	})
	return []Track{opus, aac}
}

func TestBuildTrackKeepsDistinctSampleDomainStarts(t *testing.T) {
	tracks := testTracks()
	opus, aac := tracks[0], tracks[1]

	if got, want := opus.Packets[0].RawTimestampNS, int64(50_000_000); got != want {
		t.Fatalf("Opus raw first packet = %d ns, want %d", got, want)
	}
	if got, want := ActualFrameTimestamp(opus.Packets[0], opus), int64(43_500_000); got != want {
		t.Fatalf("Opus actual first frame = %d ns, want %d", got, want)
	}
	if got, want := opus.CodecDelayNS, int64(6_500_000); got != want {
		t.Fatalf("Opus CodecDelay = %d, want %d", got, want)
	}
	if got, want := opus.SeekPreRollNS, int64(80_000_000); got != want {
		t.Fatalf("Opus SeekPreRoll = %d, want %d", got, want)
	}

	if got, want := aac.Packets[0].RawTimestampNS, int64(15_986_394); got != want {
		t.Fatalf("AAC raw first packet = %d ns, want %d", got, want)
	}
	if got, want := aac.CodecDelayNS, int64(47_891_156); got != want {
		t.Fatalf("AAC CodecDelay = %d, want %d", got, want)
	}
	actualRational := ActualFrameTimestampRational(aac.Packets[0], aac)
	wantRational := big.NewRat(-1_407*1_000_000_000, 44_100)
	if actualRational.Cmp(wantRational) != 0 {
		t.Fatalf("AAC rational first frame = %v, want %v", actualRational, wantRational)
	}
	if got, want := ActualFrameTimestamp(aac.Packets[0], aac), int64(-31_904_762); got != want {
		t.Fatalf("AAC truncated actual first frame = %d, want %d", got, want)
	}
}

func TestInitialDecoderDiscardsPrimingOnly(t *testing.T) {
	tracks := testTracks()

	opusOutput := NewDecoder(tracks[0]).DecodeFromStart()
	if got, want := opusOutput[0].Sample, int64(2_088); got != want {
		t.Fatalf("first decoded Opus sample = %d, want %d", got, want)
	}
	if !opusOutput[0].Preroll {
		t.Fatal("first Opus output must be marked priming")
	}
	visible := VisibleSamples(opusOutput)
	if got, want := visible[0].Sample, tracks[0].Spec.FirstOutputSample; got != want {
		t.Fatalf("first audible Opus sample = %d, want %d", got, want)
	}
	if got, want := visible[0].PacketSequence, 0; got != want {
		t.Fatalf("first audible Opus packet = %d, want %d", got, want)
	}

	aacOutput := NewDecoder(tracks[1]).DecodeFromStart()
	if got, want := aacOutput[0].Sample, int64(-1_407); got != want {
		t.Fatalf("first decoded AAC sample = %d, want %d", got, want)
	}
	visible = VisibleSamples(aacOutput)
	if got, want := visible[0].Sample, tracks[1].Spec.FirstOutputSample; got != want {
		t.Fatalf("first audible AAC sample = %d, want %d", got, want)
	}
	if got, want := visible[0].PacketSequence, 2; got != want {
		t.Fatalf("first audible AAC packet = %d, want %d", got, want)
	}
	if got, want := len(VisibleSamples(aacOutput[:2_112])), 0; got != want {
		t.Fatalf("visible samples before priming = %d, want 0", got)
	}
}

func TestTimelineContainsEditorialStartButNotSeekPreroll(t *testing.T) {
	tracks := testTracks()
	for _, track := range tracks {
		first := track.Packets[0]
		actual := ActualFrameTimestamp(first, track)
		withPreroll := actual - track.SeekPreRollNS

		if withPreroll == actual {
			t.Fatalf("track %d has no nonzero SeekPreRoll", track.Spec.ID)
		}
		if first.RawTimestampSample != track.Spec.FirstOutputSample {
			t.Fatalf("track %d raw timeline sample = %d, want editorial start %d", track.Spec.ID, first.RawTimestampSample, track.Spec.FirstOutputSample)
		}
		if got, want := first.RawTimestampSample-track.Spec.CodecDelaySamples, first.DecodeStartSample; got != want {
			t.Fatalf("track %d actual frame sample = %d, want decode start %d", track.Spec.ID, got, want)
		}

		decoder := NewDecoder(track)
		samples, fetchPacket, err := decoder.SeekTo(track.Spec.FirstOutputSample)
		if err != nil {
			t.Fatal(err)
		}
		if fetchPacket.Sequence != 0 && track.Spec.ID == 1 {
			t.Fatalf("track %d seek at start fetched packet %d, want 0", track.Spec.ID, fetchPacket.Sequence)
		}
		if fetchPacket.Sequence != 1 && track.Spec.ID == 2 {
			t.Fatalf("track %d seek at start fetched packet %d, want 1", track.Spec.ID, fetchPacket.Sequence)
		}
		if VisibleSamples(samples)[0].Sample != track.Spec.FirstOutputSample {
			t.Fatalf("track %d moved visible timeline after seek", track.Spec.ID)
		}
	}
}
