package channelmap

import (
	"bytes"
	"math"
	"testing"
)

func TestParseExtensible16Bit51(t *testing.T) {
	channels := Standard51ToneChannels(0.05, 48000)
	data, err := EncodeWAV(channels, EncodeWAVOptions{SampleRate: 48000, BitsPerSample: 16, ChannelMask: Mask5Point1})
	if err != nil {
		t.Fatal(err)
	}
	f, err := Parse(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if f.Format.Channels != 6 || f.Format.SampleRate != 48000 || f.Frames != 2400 {
		t.Fatalf("format/frames = %+v frames=%d", f.Format, f.Frames)
	}
	if !f.Format.HasChannelMask || f.Format.ChannelMask != Mask5Point1 {
		t.Fatalf("mask = %#v", f.Format)
	}
	if f.Format.Codec.ID != "pcm_s16le" || !f.Format.Codec.Decodable {
		t.Fatalf("codec = %+v", f.Format.Codec)
	}
	if len(f.Samples) != 6 || len(f.Samples[0]) != 2400 {
		t.Fatalf("decoded channels = %d", len(f.Samples))
	}
	if math.Abs(f.Samples[3][1000]-channels[3][1000]) > 0.001 {
		t.Fatalf("16-bit sample decode mismatch: got %.6f want %.6f", f.Samples[3][1000], channels[3][1000])
	}
}

func TestParseFloat32And24Bit(t *testing.T) {
	for _, tc := range []struct {
		name  string
		opts  EncodeWAVOptions
		delta float64
	}{
		{"float32", EncodeWAVOptions{SampleRate: 32000, IEEEFloat: true, ChannelMask: MaskStereo}, 1e-6},
		{"s24", EncodeWAVOptions{SampleRate: 32000, BitsPerSample: 24, ChannelMask: MaskStereo}, 1e-5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			channels := [][]float64{ToneChannel(440, 0.03, 32000, 0.3), ToneChannel(550, 0.03, 32000, 0.3)}
			data, err := EncodeWAV(channels, tc.opts)
			if err != nil {
				t.Fatal(err)
			}
			f, err := Parse(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			if len(f.Samples) != 2 {
				t.Fatalf("channels = %d", len(f.Samples))
			}
			if math.Abs(f.Samples[0][100]-channels[0][100]) > tc.delta {
				t.Fatalf("sample mismatch got %.9f want %.9f", f.Samples[0][100], channels[0][100])
			}
		})
	}
}

func TestParseRejectsNonWAV(t *testing.T) {
	_, err := Parse(bytes.NewReader([]byte("not a wav file at all but long enough")))
	if err == nil {
		t.Fatal("Parse() succeeded for non-WAV input")
	}
}
