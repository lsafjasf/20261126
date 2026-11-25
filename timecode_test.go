package sdh

import (
	"testing"
	"time"
)

func TestParseTimestamp(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"00:00:01,000", time.Second},
		{"00:01:02,500", 62*time.Second + 500*time.Millisecond},
		{"01:02:03,25", time.Hour + 2*time.Minute + 3*time.Second + 250*time.Millisecond},
		{"00:00:01.500", 1500 * time.Millisecond},
		{"02:03", 2*time.Minute + 3*time.Second},
		{" 00:00:00,010 ", 10 * time.Millisecond},
	}
	for _, tc := range cases {
		got, err := ParseTimestamp(tc.in)
		if err != nil {
			t.Fatalf("ParseTimestamp(%q) error: %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("ParseTimestamp(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestParseTimestampErrors(t *testing.T) {
	for _, in := range []string{"", "abc", "1:2:3:4", "00:61:00,000", "00:00:60,000"} {
		if _, err := ParseTimestamp(in); err == nil {
			t.Fatalf("ParseTimestamp(%q) expected error", in)
		}
	}
}

func TestTimestampRoundTrip(t *testing.T) {
	d := 3661500 * time.Millisecond // 01:01:01,500
	s := FormatSRTTime(d)
	if s != "01:01:01,500" {
		t.Fatalf("FormatSRTTime = %q", s)
	}
	back, err := ParseTimestamp(s)
	if err != nil || back != d {
		t.Fatalf("round trip = %v, %v; want %v", back, err, d)
	}
	if ass := FormatASSTime(d); ass != "1:01:01.50" {
		t.Fatalf("FormatASSTime = %q", ass)
	}
}
