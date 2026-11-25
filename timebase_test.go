package subselect

import (
	"testing"
	"time"
)

func TestTimeBase_ToDuration(t *testing.T) {
	cases := []struct {
		name string
		tb   TimeBase
		in   int64
		want time.Duration
	}{
		{"millisecond base", TimeBase{Timescale: 1000}, 4500, 4500 * time.Millisecond},
		{"90k base", TimeBase{Timescale: 90000}, 405000, 4500 * time.Millisecond},
		{"48k base", TimeBase{Timescale: 48000}, 216000, 4500 * time.Millisecond},
		{"fractional remainder", TimeBase{Timescale: 48000}, 1, time.Second / 48000},
		{"offset shifts", TimeBase{Timescale: 1000, Offset: 500 * time.Millisecond}, 1200, 1700 * time.Millisecond},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.tb.ToDuration(tc.in); got != tc.want {
				t.Fatalf("ToDuration(%d) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestTimeBase_RoundTrip(t *testing.T) {
	tb := TimeBase{Timescale: 90000, Offset: 250 * time.Millisecond}
	for _, ms := range []int64{0, 1, 4500, 12345, 99999} {
		pos := time.Duration(ms) * time.Millisecond
		units := tb.FromDuration(pos)
		back := tb.ToDuration(units)
		// Round trip through integer units may lose < 1 unit of precision.
		if delta := absDuration(back - pos); delta > time.Second/time.Duration(tb.Timescale)+time.Millisecond {
			t.Fatalf("round trip for %v drifted %v (units=%d back=%v)", pos, delta, units, back)
		}
	}
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}
