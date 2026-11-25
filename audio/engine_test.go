package audio

import (
	"testing"
	"time"
)

var caps441 = Capabilities{
	SampleRates: []int{44100},
	Layouts:     []ChannelLayout{LayoutStereo},
	Formats:     []SampleFormat{FormatS16},
}

var caps48k = Capabilities{
	SampleRates: []int{48000},
	Layouts:     []ChannelLayout{LayoutStereo},
	Formats:     []SampleFormat{FormatS16},
}

func waitForState(t *testing.T, e *Engine, want State) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if e.State() == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("state did not become %v (now %v)", want, e.State())
}

// The reported bug: device switches 44.1 -> 48 kHz mid-stream. The engine
// must renegotiate instead of keeping the stale layout/rate, and the sample
// position must stay continuous.
func TestRateSwitchRenegotiates(t *testing.T) {
	dev := NewSimDevice(caps441)
	eng := NewEngine(StreamParams{SampleRate: 44100, Layout: LayoutStereo, Format: FormatS16})
	defer eng.Close()
	if err := eng.Attach(dev); err != nil {
		t.Fatal(err)
	}
	waitForState(t, eng, StateRunning)

	// Render 1 second at 44.1 kHz.
	if err := eng.Render(44100); err != nil {
		t.Fatal(err)
	}

	dev.SetCaps(caps48k)
	waitForState(t, eng, StateDegraded) // 44.1k desired, 48k selected

	neg := eng.Negotiation()
	if neg.Selected.SampleRate != 48000 {
		t.Fatalf("expected renegotiation to 48 kHz, got %v", neg.Selected)
	}
	if len(neg.Fallbacks) != 1 || neg.Fallbacks[0].Field != "sample_rate" {
		t.Fatalf("expected sample_rate fallback, got %+v", neg.Fallbacks)
	}

	// Render 1 second at 48 kHz; canonical position must be ~2 s of source.
	if err := eng.Render(48000); err != nil {
		t.Fatal(err)
	}
	src, _, total := eng.Position()
	if src != 88200 {
		t.Fatalf("source position = %d, want 88200", src)
	}
	if total != 44100+48000 {
		t.Fatalf("total device frames = %d", total)
	}

	anchors := eng.Anchors()
	if len(anchors) != 2 {
		t.Fatalf("expected 2 anchors, got %d", len(anchors))
	}
	sw := anchors[1]
	if sw.SourceFrame != 44100 || sw.DeviceRate != 48000 {
		t.Fatalf("switch anchor = %+v", sw)
	}
}

func TestDeviceRemovedAndRestored(t *testing.T) {
	dev := NewSimDevice(caps441)
	eng := NewEngine(StreamParams{SampleRate: 44100, Layout: LayoutStereo, Format: FormatS16})
	defer eng.Close()
	if err := eng.Attach(dev); err != nil {
		t.Fatal(err)
	}
	waitForState(t, eng, StateRunning)
	if err := eng.Render(22050); err != nil {
		t.Fatal(err)
	}

	dev.Remove()
	waitForState(t, eng, StateDeviceLost)

	// Rendering while the device is gone must fail and not move position.
	if err := eng.Render(1000); err != ErrNotRunning {
		t.Fatalf("render on lost device: %v", err)
	}
	src, _, _ := eng.Position()
	if src != 22050 {
		t.Fatalf("position moved while device lost: %d", src)
	}

	// Device comes back as 48 kHz only: recover degraded, position intact.
	dev.Restore(caps48k)
	waitForState(t, eng, StateDegraded)
	src, _, _ = eng.Position()
	if src != 22050 {
		t.Fatalf("position lost across recovery: %d", src)
	}
	if err := eng.Render(48000); err != nil {
		t.Fatal(err)
	}
	src, _, _ = eng.Position()
	if src != 22050+44100 {
		t.Fatalf("position after recovery render = %d", src)
	}
}

func TestDevicePermanentlyGone(t *testing.T) {
	dev := NewSimDevice(caps441)
	eng := NewEngine(StreamParams{SampleRate: 44100, Layout: LayoutStereo, Format: FormatS16})
	defer eng.Close()
	if err := eng.Attach(dev); err != nil {
		t.Fatal(err)
	}
	waitForState(t, eng, StateRunning)
	dev.Gone()
	waitForState(t, eng, StateDeviceLost)
}

func TestAttachTwiceFails(t *testing.T) {
	dev := NewSimDevice(caps441)
	eng := NewEngine(StreamParams{SampleRate: 44100, Layout: LayoutStereo, Format: FormatS16})
	defer eng.Close()
	if err := eng.Attach(dev); err != nil {
		t.Fatal(err)
	}
	if err := eng.Attach(dev); err != ErrAttached {
		t.Fatalf("want ErrAttached, got %v", err)
	}
}

func TestOnChangeCallback(t *testing.T) {
	dev := NewSimDevice(caps441)
	eng := NewEngine(StreamParams{SampleRate: 44100, Layout: LayoutStereo, Format: FormatS16})
	defer eng.Close()
	seen := make(chan State, 8)
	eng.OnChange = func(s State, n Negotiation) { seen <- s }
	if err := eng.Attach(dev); err != nil {
		t.Fatal(err)
	}
	select {
	case s := <-seen:
		if s != StateRunning {
			t.Fatalf("first callback state = %v", s)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no OnChange callback")
	}
}
