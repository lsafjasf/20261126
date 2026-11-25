package channelmap

import "testing"

func TestReorderChannelsUsesSourceIndices(t *testing.T) {
	original := Standard51ToneChannels(0.08, 48000)
	swapped := SwapCenterBackLeft(original)
	r := analyzeScenario(t, swapped, Options{UseCalibrationProfile: true})
	if r.Repair == nil {
		t.Fatal("expected repair order")
	}
	fixed, err := ReorderChannels(swapped, r.Repair.SourceIndicesForMaskOrder)
	if err != nil {
		t.Fatal(err)
	}
	if len(fixed) != 6 {
		t.Fatalf("channels = %d", len(fixed))
	}

	data, err := EncodeWAV(fixed, EncodeWAVOptions{SampleRate: 48000, BitsPerSample: 16, ChannelMask: Mask5Point1})
	if err != nil {
		t.Fatal(err)
	}
	repaired, err := AnalyzeBytes(data, Options{UseCalibrationProfile: true})
	if err != nil {
		t.Fatal(err)
	}
	if repaired.Status != StatusConsistent {
		t.Fatalf("repaired status = %s, conflicts=%+v", repaired.Status, repaired.Conflicts)
	}
}

func TestReorderChannelsRejectsInvalidPermutation(t *testing.T) {
	channels := make([][]float64, 3)
	if _, err := ReorderChannels(channels, []int{0, 0, 2}); err == nil {
		t.Fatal("expected duplicate source index to fail")
	}
	if _, err := ReorderChannels(channels, []int{0, 1, 3}); err == nil {
		t.Fatal("expected out-of-range source index to fail")
	}
}
