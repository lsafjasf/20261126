package channelmap

import (
	"os"
	"testing"
)

func analyzeScenario(t *testing.T, channels [][]float64, opts Options) *Report {
	t.Helper()
	data, err := EncodeWAV(channels, EncodeWAVOptions{SampleRate: 48000, BitsPerSample: 16, ChannelMask: Mask5Point1})
	if err != nil {
		t.Fatal(err)
	}
	r, err := AnalyzeBytes(data, opts)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestCorrect51IsConsistentWithCalibrationProfile(t *testing.T) {
	r := analyzeScenario(t, Standard51ToneChannels(0.12, 48000), Options{UseCalibrationProfile: true})
	if r.Status != StatusConsistent {
		t.Fatalf("status = %s, want consistent; conflicts=%+v undecidable=%+v", r.Status, r.Conflicts, r.Undecidable)
	}
	if r.Confidence != ConfidenceHigh {
		t.Fatalf("confidence = %s, want high", r.Confidence)
	}
	for _, m := range r.InferredMapping {
		if m.Status != "match" {
			t.Fatalf("wire %d status = %s, want match", m.WireIndex, m.Status)
		}
	}
}

func TestDetectsCenterSurroundSwapAndRepairOrder(t *testing.T) {
	channels := SwapCenterBackLeft(Standard51ToneChannels(0.12, 48000))
	r := analyzeScenario(t, channels, Options{UseCalibrationProfile: true})
	if r.Status != StatusConflict {
		t.Fatalf("status = %s, want conflict", r.Status)
	}
	if len(r.Conflicts) != 1 || r.Conflicts[0].Code != "CENTER_SURROUND_SWAP" {
		t.Fatalf("conflicts = %+v, want one CENTER_SURROUND_SWAP", r.Conflicts)
	}
	c := r.Conflicts[0]
	if c.Confidence != ConfidenceHigh || len(c.WireIndices) != 2 || c.WireIndices[0] != 2 || c.WireIndices[1] != 4 {
		t.Fatalf("conflict = %+v, want high-confidence wires [2 4]", c)
	}
	if r.Repair == nil {
		t.Fatal("repair mapping is nil")
	}
	wantOrder := []int{0, 1, 4, 3, 2, 5}
	if len(r.Repair.SourceIndicesForMaskOrder) != len(wantOrder) {
		t.Fatalf("repair order = %v, want %v", r.Repair.SourceIndicesForMaskOrder, wantOrder)
	}
	for i := range wantOrder {
		if r.Repair.SourceIndicesForMaskOrder[i] != wantOrder[i] {
			t.Fatalf("repair order = %v, want %v", r.Repair.SourceIndicesForMaskOrder, wantOrder)
		}
	}
}

func TestSurroundSideRemainsUndecidableButFamilySwapIsReported(t *testing.T) {
	f, err := os.Open("testdata/surround-family-signatures.json")
	if err != nil {
		t.Fatal(err)
	}
	sig, err := LoadSignatures(f)
	_ = f.Close()
	if err != nil {
		t.Fatal(err)
	}

	r := analyzeScenario(t, AmbiguousCenterSurroundChannels(0.12, 48000), Options{Signatures: sig})
	if r.Status != StatusConflict {
		t.Fatalf("status = %s, want conflict", r.Status)
	}
	var foundSwap, foundAmbiguity bool
	for _, c := range r.Conflicts {
		if c.Code == "CENTER_SURROUND_SWAP" && c.Confidence == ConfidenceMedium {
			foundSwap = true
		}
	}
	for _, u := range r.Undecidable {
		if u.Code == "CENTER_OR_SURROUND_SIDE_AMBIGUOUS" {
			foundAmbiguity = true
		}
	}
	if !foundSwap {
		t.Fatalf("conflicts = %+v, want medium family swap", r.Conflicts)
	}
	if !foundAmbiguity {
		t.Fatalf("undecidable = %+v, want side ambiguity", r.Undecidable)
	}
}

func TestWithoutSignaturesReportsUndecidableIdentity(t *testing.T) {
	r := analyzeScenario(t, Standard51ToneChannels(0.1, 48000), Options{})
	if r.Status != StatusUndecidable {
		t.Fatalf("status = %s, want undecidable", r.Status)
	}
	found := false
	for _, u := range r.Undecidable {
		if u.Code == "SPEAKER_IDENTITY_NOT_ENCODED" {
			found = true
		}
	}
	if !found {
		t.Fatalf("undecidable = %+v, want SPEAKER_IDENTITY_NOT_ENCODED", r.Undecidable)
	}
	if len(r.Features) != 6 || r.Features[2].DominantFrequencyHz < 900 {
		t.Fatalf("features not extracted: %+v", r.Features)
	}
}

func TestAutoProfileDetectsBundledCalibrationSwap(t *testing.T) {
	channels := SwapCenterBackLeft(Standard51ToneChannels(0.12, 48000))
	r := analyzeScenario(t, channels, Options{Profile: "auto"})
	if r.Status != StatusConflict {
		t.Fatalf("status = %s, want conflict: %+v", r.Status, r.Undecidable)
	}
	if len(r.Conflicts) == 0 || r.Conflicts[0].Code != "CENTER_SURROUND_SWAP" {
		t.Fatalf("conflicts = %+v", r.Conflicts)
	}
}

func TestMaskChannelCountMismatchIsConflict(t *testing.T) {
	channels := Standard51ToneChannels(0.08, 48000)
	data, err := EncodeWAV(channels, EncodeWAVOptions{SampleRate: 48000, BitsPerSample: 16, ChannelMask: MaskStereo})
	if err != nil {
		t.Fatal(err)
	}
	r, err := AnalyzeBytes(data, Options{UseCalibrationProfile: true})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range r.Conflicts {
		if c.Code == "MASK_CHANNEL_COUNT_MISMATCH" {
			found = true
		}
	}
	if !found {
		t.Fatalf("conflicts = %+v, want mask count mismatch", r.Conflicts)
	}
}

func TestPlainStereoMaskAbsenceIsUndecidableMetadata(t *testing.T) {
	channels := [][]float64{ToneChannel(330, 0.05, 48000, 0.2), ToneChannel(440, 0.05, 48000, 0.2)}
	data, err := EncodeWAV(channels, EncodeWAVOptions{SampleRate: 48000, BitsPerSample: 16, Extensible: false})
	if err != nil {
		t.Fatal(err)
	}
	r, err := AnalyzeBytes(data, Options{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, u := range r.Undecidable {
		if u.Code == "MASK_NOT_DECLARED" {
			found = true
		}
	}
	if !found {
		t.Fatalf("undecidable = %+v, want MASK_NOT_DECLARED", r.Undecidable)
	}
}
