package ttmlevents

import (
	"testing"
	"time"
)

func TestParseTimeForms(t *testing.T) {
	cases := []struct {
		raw  string
		want time.Duration
	}{
		{"1.5s", 1500 * time.Millisecond},
		{"250ms", 250 * time.Millisecond},
		{"2m", 2 * time.Minute},
		{"1h", time.Hour},
		{"00:00:02.500", 2500 * time.Millisecond},
		{"00:01:02", 62 * time.Second},
		{"01:02:03", time.Hour + 2*time.Minute + 3*time.Second},
		{"00:00:01:15", 1500 * time.Millisecond}, // default 30 fps
	}
	for _, tc := range cases {
		got, err := ParseTime(tc.raw)
		if err != nil {
			t.Fatalf("ParseTime(%q) error: %v", tc.raw, err)
		}
		if got != tc.want {
			t.Fatalf("ParseTime(%q) = %v, want %v", tc.raw, got, tc.want)
		}
	}
}

func TestNestedTimingIsRelativeToTimeContainer(t *testing.T) {
	doc := mustParse(t, `<tt><body><p begin="1s" end="6s">
		<span id="outer" begin="0s" end="4s">
			<span id="inner" begin="1s" end="2s">x</span>
		</span>
	</p></body></tt>`)
	tl := ResolveTiming(doc)

	outer := timingByID(t, tl, "outer")
	if outer.Effective.Begin != time.Second || endOrZero(outer.Effective) != 5*time.Second {
		t.Fatalf("outer = [%v,%v), want [1s,5s)", outer.Effective.Begin, endOrZero(outer.Effective))
	}
	inner := timingByID(t, tl, "inner")
	if inner.Effective.Begin != 2*time.Second || endOrZero(inner.Effective) != 3*time.Second {
		t.Fatalf("inner = [%v,%v), want [2s,3s)", inner.Effective.Begin, endOrZero(inner.Effective))
	}
}

func TestParentEndClipsChild(t *testing.T) {
	doc := mustParse(t, `<tt><body><p begin="1s" end="3s">
		<span id="child" begin="0s" dur="5s">x</span>
	</p></body></tt>`)
	tl := ResolveTiming(doc)
	child := timingByID(t, tl, "child")
	if !child.Clipped {
		t.Fatal("expected child interval to be clipped")
	}
	if endOrZero(child.Effective) != 3*time.Second {
		t.Fatalf("child end = %v, want 3s", endOrZero(child.Effective))
	}
}

func mustParse(t *testing.T, xml string) *Document {
	t.Helper()
	doc, err := ParseString(xml)
	if err != nil {
		t.Fatalf("ParseString error: %v\nxml=%s", err, xml)
	}
	return doc
}

func timingByID(t *testing.T, tl *Timeline, id string) *NodeTiming {
	t.Helper()
	for _, info := range tl.Timings {
		if info.ID == id {
			return info
		}
	}
	t.Fatalf("timing id %q not found", id)
	return nil
}

func endOrZero(interval Interval) time.Duration {
	if interval.End == nil {
		return 0
	}
	return *interval.End
}
