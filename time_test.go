package smptett

import (
	"testing"
	"time"
)

func TestParseTTMLTime(t *testing.T) {
	tests := []struct {
		value string
		fps   float64
		tick  float64
		want  time.Duration
	}{
		{"1.5s", 25, 25, 1500 * time.Millisecond},
		{"250ms", 25, 25, 250 * time.Millisecond},
		{"00:00:02.750", 25, 25, 2750 * time.Millisecond},
		{"00:00:01:05", 25, 25, 1200 * time.Millisecond},
		{"10f", 25, 25, 400 * time.Millisecond},
		{"100t", 25, 100, time.Second},
	}
	for _, test := range tests {
		got, err := parseTTMLTime(test.value, test.fps, test.tick)
		if err != nil {
			t.Fatalf("parseTTMLTime(%q): %v", test.value, err)
		}
		if got != test.want {
			t.Errorf("parseTTMLTime(%q) = %s, want %s", test.value, got, test.want)
		}
	}
}

func TestParseTTMLTimeRejectsInvalid(t *testing.T) {
	for _, value := range []string{"", "-1s", "01:99:00.000", "not-a-time"} {
		if _, err := parseTTMLTime(value, 25, 25); err == nil {
			t.Errorf("parseTTMLTime(%q) accepted invalid value", value)
		}
	}
}

func TestPreviewTimesUsesCueBoundaries(t *testing.T) {
	doc := &Document{
		Duration: 4 * time.Second,
		Resources: []*Resource{
			{Begin: 0, End: 2 * time.Second},
			{Begin: time.Second, End: 3 * time.Second},
		},
	}
	times, err := PreviewTimes(doc, PreviewOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []time.Duration{0, time.Second, 2*time.Second - time.Nanosecond, 3*time.Second - time.Nanosecond} {
		if !containsDuration(times, want) {
			t.Errorf("preview times %v do not contain transition %s", times, want)
		}
	}
	if len(times) > 200 {
		t.Fatalf("unbounded preview times: %d", len(times))
	}
}

func TestPreviewTimesLimitsAndFPS(t *testing.T) {
	doc := &Document{Duration: 10 * time.Second}
	times, err := PreviewTimes(doc, PreviewOptions{FPS: 4, MaxFrames: 12})
	if err != nil {
		t.Fatal(err)
	}
	if len(times) != 12 {
		t.Fatalf("got %d times, want 12", len(times))
	}
	for i := 1; i < len(times); i++ {
		if times[i] <= times[i-1] {
			t.Fatalf("times are not strictly increasing: %v", times)
		}
	}
}

func containsDuration(values []time.Duration, want time.Duration) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
