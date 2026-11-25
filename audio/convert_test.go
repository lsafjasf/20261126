package audio

import "testing"

func TestDownmix51ToStereo(t *testing.T) {
	// One 5.1 frame: FL FR FC LFE BL BR.
	src := []float32{1, 1, 1, 0, 1, 1}
	dst := RemapLayout(src, LayoutSurround5, LayoutStereo)
	if len(dst) != 2 {
		t.Fatalf("want 2 samples, got %d", len(dst))
	}
	// L = FL + 0.707*FC + 0.707*BL
	want := float32(1 + 0.707 + 0.707)
	if dst[0] < want-1e-3 || dst[0] > want+1e-3 {
		t.Fatalf("L = %v, want ~%v", dst[0], want)
	}
}

func TestUpmixStereoToMonoAndBack(t *testing.T) {
	src := []float32{1, 0, 0, 1} // two stereo frames
	mono := RemapLayout(src, LayoutStereo, LayoutMono)
	if len(mono) != 2 || mono[0] != 0.5 || mono[1] != 0.5 {
		t.Fatalf("mono = %v", mono)
	}
	back := RemapLayout(mono, LayoutMono, LayoutStereo)
	if len(back) != 4 || back[0] != 0.5 || back[1] != 0.5 {
		t.Fatalf("stereo = %v", back)
	}
}

func TestResampleFrameCount(t *testing.T) {
	src := make([]float32, 44100*2) // 1 s stereo @ 44.1k
	dst := ResampleLinear(src, 2, 44100, 48000)
	if got := len(dst) / 2; got != 48000 {
		t.Fatalf("got %d frames, want 48000", got)
	}
}

func TestConvertMatchesNegotiatedParams(t *testing.T) {
	from := StreamParams{SampleRate: 44100, Layout: LayoutSurround5, Format: FormatF32}
	to := StreamParams{SampleRate: 48000, Layout: LayoutStereo, Format: FormatF32}
	src := make([]float32, 4410*6) // 0.1 s of 5.1
	dst := Convert(src, from, to)
	if got := len(dst) / 2; got != 4800 {
		t.Fatalf("got %d stereo frames, want 4800", got)
	}
}
