package imsc

import (
	"testing"
	"time"
)

func TestParseClock(t *testing.T) {
	cases := []struct {
		expr string
		want time.Duration
	}{
		{"0s", 0},
		{"1.5s", 1500 * time.Millisecond},
		{"200ms", 200 * time.Millisecond},
		{"2m", 2 * time.Minute},
		{"1h", time.Hour},
		{"00:00:01", time.Second},
		{"01:02:03.5", time.Hour + 2*time.Minute + 3500*time.Millisecond},
		{"00:00:01:15", time.Second + 500*time.Millisecond},
		{"00:01:00,0", time.Minute},
	}
	for _, tc := range cases {
		got, err := parseClock(tc.expr)
		if err != nil {
			t.Fatalf("parseClock(%q) unexpected error: %v", tc.expr, err)
		}
		if got != tc.want {
			t.Fatalf("parseClock(%q) = %v, want %v", tc.expr, got, tc.want)
		}
	}
}

func TestParseClockErrors(t *testing.T) {
	bad := []string{"", "  ", "12", "abc", "00:60:00", "00:00:60", "00:00:01:30", "1..5s", "s"}
	for _, expr := range bad {
		if _, err := parseClock(expr); err == nil {
			t.Fatalf("parseClock(%q) expected error, got nil", expr)
		}
	}
}

func TestFragmentID(t *testing.T) {
	id, ok := fragmentID("#img1")
	if !ok || id != "img1" {
		t.Fatalf("fragmentID(#img1) = %q,%v", id, ok)
	}
	for _, ref := range []string{"img1", "#", "# img", "http://x/y.png", ""} {
		if _, ok := fragmentID(ref); ok {
			t.Fatalf("fragmentID(%q) expected rejection", ref)
		}
	}
}

func TestCustomFrameRate(t *testing.T) {
	d, err := parseClockWithRate("00:00:01:10", 25)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := time.Second + 400*time.Millisecond
	if d != want {
		t.Fatalf("frame-based clock = %v, want %v", d, want)
	}
	if _, err := parseClockWithRate("00:00:01:25", 25); err == nil {
		t.Fatalf("frame equal to frame rate must be rejected")
	}
}
