package audio

import "testing"

func TestNegotiateExact(t *testing.T) {
	caps := Capabilities{
		SampleRates: []int{44100, 48000},
		Layouts:     []ChannelLayout{LayoutStereo},
		Formats:     []SampleFormat{FormatS16, FormatF32},
	}
	want := StreamParams{SampleRate: 44100, Layout: LayoutStereo, Format: FormatF32}
	n, err := Negotiate(want, caps)
	if err != nil {
		t.Fatal(err)
	}
	if !n.Exact() {
		t.Fatalf("expected exact negotiation, got fallbacks %+v", n.Fallbacks)
	}
	if n.Selected != want {
		t.Fatalf("selected %v, want %v", n.Selected, want)
	}
}

func TestNegotiatePartialSupport(t *testing.T) {
	caps := Capabilities{
		SampleRates: []int{48000},
		Layouts:     []ChannelLayout{LayoutStereo},
		Formats:     []SampleFormat{FormatS16},
	}
	want := StreamParams{SampleRate: 96000, Layout: LayoutSurround5, Format: FormatF32}
	n, err := Negotiate(want, caps)
	if err != nil {
		t.Fatal(err)
	}
	if n.Exact() {
		t.Fatal("expected partial support")
	}
	if n.Selected.SampleRate != 48000 || n.Selected.Layout != LayoutStereo || n.Selected.Format != FormatS16 {
		t.Fatalf("unexpected selection %v", n.Selected)
	}
	if len(n.Fallbacks) != 3 {
		t.Fatalf("expected 3 fallbacks, got %+v", n.Fallbacks)
	}
}

func TestNegotiatePrefersDownmixOverUpmix(t *testing.T) {
	caps := Capabilities{
		SampleRates: []int{48000},
		Layouts:     []ChannelLayout{LayoutStereo, LayoutSurround5},
		Formats:     []SampleFormat{FormatS16},
	}
	want := StreamParams{SampleRate: 48000, Layout: LayoutQuad, Format: FormatS16}
	n, err := Negotiate(want, caps)
	if err != nil {
		t.Fatal(err)
	}
	if n.Selected.Layout != LayoutStereo {
		t.Fatalf("expected downmix to stereo, got %v", n.Selected.Layout)
	}
}

func TestNegotiateUnusableDevice(t *testing.T) {
	_, err := Negotiate(
		StreamParams{SampleRate: 44100, Layout: LayoutStereo, Format: FormatS16},
		Capabilities{},
	)
	if err != ErrNoUsableDevice {
		t.Fatalf("want ErrNoUsableDevice, got %v", err)
	}
}

func TestNearestRateTiePrefersHigher(t *testing.T) {
	if got := nearestRate(46050, []int{44100, 48000}); got != 48000 {
		t.Fatalf("got %d", got)
	}
}
