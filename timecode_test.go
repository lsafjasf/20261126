package timeline

import (
	"math/rand"
	"testing"
)

func mustRate(t *testing.T, num, den int64) Rate {
	t.Helper()
	rate, err := NewRate(num, den)
	if err != nil {
		t.Fatalf("NewRate(%d, %d): %v", num, den, err)
	}
	return rate
}

func TestTimecodeKnownBoundaries(t *testing.T) {
	ntsc := mustRate(t, 30000, 1001)
	tests := []struct {
		frame int64
		want  string
	}{
		{0, "00:00:00;00"},
		{1799, "00:00:59;29"},
		{1800, "00:01:00;02"},
		{19781, "00:10:59;29"},
		{19782, "00:11:00;02"},
		{17982, "00:10:00;00"},
		{17983, "00:10:00;01"},
		{35964, "00:20:00;00"},
		{53946, "00:30:00;00"},
	}
	for _, tt := range tests {
		got, err := FrameToTimecode(ntsc, tt.frame)
		if err != nil {
			t.Fatalf("FrameToTimecode(%d): %v", tt.frame, err)
		}
		if got != tt.want {
			t.Errorf("FrameToTimecode(%d) = %q, want %q", tt.frame, got, tt.want)
		}
		frame, err := ParseTimecode(ntsc, got)
		if err != nil {
			t.Fatalf("ParseTimecode(%q): %v", got, err)
		}
		if frame != tt.frame {
			t.Errorf("ParseTimecode(%q) = %d, want %d", got, frame, tt.frame)
		}
	}

	doubleNTSC := mustRate(t, 60000, 1001)
	got, err := FrameToTimecode(doubleNTSC, 35964)
	if err != nil {
		t.Fatal(err)
	}
	if got != "00:10:00;00" {
		t.Errorf("59.94 tenth-minute timecode = %q, want %q", got, "00:10:00;00")
	}
}

func TestTimecodeRejectsDroppedLabels(t *testing.T) {
	ntsc := mustRate(t, 30000, 1001)
	for _, tc := range []string{"00:01:00;00", "00:01:00;01", "00:19:00;01"} {
		if _, err := ParseTimecode(ntsc, tc); err == nil {
			t.Errorf("ParseTimecode(%q) accepted a dropped label", tc)
		}
	}
	for _, tc := range []string{"00:10:00;00", "00:10:00;01", "00:20:00;00"} {
		if _, err := ParseTimecode(ntsc, tc); err != nil {
			t.Errorf("ParseTimecode(%q): %v", tc, err)
		}
	}
}

func TestTimecodeRoundTripRandom(t *testing.T) {
	rates := []Rate{
		mustRate(t, 30000, 1001),
		mustRate(t, 60000, 1001),
		mustRate(t, 24000, 1001),
		mustRate(t, 25, 1),
		mustRate(t, 24, 1),
		mustRate(t, 30, 1),
		mustRate(t, 50, 1),
	}
	rng := rand.New(rand.NewSource(0x5eed))
	for _, rate := range rates {
		maxFrame := rate.framesPerHour() * 8
		probes := []int64{
			0, 1, rate.nominalFPS() - 1, rate.nominalFPS(),
			rate.framesPer10Minutes() - 2,
			rate.framesPer10Minutes() - 1,
			rate.framesPer10Minutes(),
			rate.framesPer10Minutes() + 1,
			rate.framesPer10Minutes() + 2,
			2 * rate.framesPer10Minutes(),
		}
		for i := 0; i < 500; i++ {
			probes = append(probes, rng.Int63n(maxFrame))
		}
		for _, frame := range probes {
			if frame < 0 || frame >= rate.framesPerDay() {
				continue
			}
			tc, err := FrameToTimecode(rate, frame)
			if err != nil {
				t.Fatalf("rate %s frame %d: %v", rate, frame, err)
			}
			got, err := ParseTimecode(rate, tc)
			if err != nil {
				t.Fatalf("rate %s timecode %q: %v", rate, tc, err)
			}
			if got != frame {
				t.Fatalf("rate %s timecode %q round trip = %d, want %d", rate, tc, got, frame)
			}
		}
	}
}
