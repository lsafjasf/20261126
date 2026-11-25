package channelmap

import (
	"bytes"
	"testing"
)

func TestReferenceTemplatesDetectSwap(t *testing.T) {
	correct := Standard51ToneChannels(0.12, 48000)
	refBytes, err := EncodeWAV(correct, EncodeWAVOptions{SampleRate: 48000, BitsPerSample: 16, ChannelMask: Mask5Point1})
	if err != nil {
		t.Fatal(err)
	}
	ref, err := Parse(bytes.NewReader(refBytes))
	if err != nil {
		t.Fatal(err)
	}
	sig, err := SignaturesFromReference(ref)
	if err != nil {
		t.Fatal(err)
	}

	swapped := SwapCenterBackLeft(Standard51ToneChannels(0.12, 48000))
	r := analyzeScenario(t, swapped, Options{Signatures: sig})
	if r.Status != StatusConflict {
		t.Fatalf("status=%s conflicts=%+v undecidable=%+v", r.Status, r.Conflicts, r.Undecidable)
	}
	found := false
	for _, c := range r.Conflicts {
		if c.Code == "CENTER_SURROUND_SWAP" && c.Confidence == ConfidenceHigh {
			found = true
		}
	}
	if !found {
		t.Fatalf("conflicts=%+v, want high-confidence swap from templates", r.Conflicts)
	}
}
