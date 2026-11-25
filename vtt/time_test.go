package vtt

import "testing"

func TestParseTimestamp(t *testing.T) {
	cases := []struct {
		input string
		want  int64
	}{
		{"00:00.000", 0},
		{"01:02.500", 62_500},
		{"00:00:01.000", 1_000},
		{"01:02:03.250", 3_723_250},
		{"01:02:03,250", 3_723_250}, // SRT-style comma accepted on import
		{"01:02:03.25", 3_723_250},  // short fraction padded
	}
	for _, tc := range cases {
		got, err := ParseTimestamp(tc.input)
		if err != nil {
			t.Fatalf("ParseTimestamp(%q) error: %v", tc.input, err)
		}
		if got != tc.want {
			t.Errorf("ParseTimestamp(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

func TestParseTimestampErrors(t *testing.T) {
	for _, input := range []string{"", "12", "60:00.000", "00:60.000", "aa:bb.ccc"} {
		if _, err := ParseTimestamp(input); err == nil {
			t.Errorf("ParseTimestamp(%q) expected error, got nil", input)
		}
	}
}

func TestFormatTimestamp(t *testing.T) {
	cases := []struct {
		millis int64
		want   string
	}{
		{0, "00:00.000"},
		{62_500, "01:02.500"},
		{3_723_250, "01:02:03.250"},
		{3_600_000, "01:00:00.000"},
	}
	for _, tc := range cases {
		if got := FormatTimestamp(tc.millis); got != tc.want {
			t.Errorf("FormatTimestamp(%d) = %q, want %q", tc.millis, got, tc.want)
		}
	}
}

func TestTimestampRoundTrip(t *testing.T) {
	for _, value := range []string{"00:00.000", "59:59.999", "12:34:56.789"} {
		parsed, err := ParseTimestamp(value)
		if err != nil {
			t.Fatalf("parse %q: %v", value, err)
		}
		if got := FormatTimestamp(parsed); got != value {
			t.Errorf("round trip = %q, want %q", got, value)
		}
	}
}
