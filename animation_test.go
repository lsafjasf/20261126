package ttmlevents

import (
	"testing"
	"time"
)

func TestAnimationKeyframesAndFreezeStyleIntervals(t *testing.T) {
	source := `<tt xmlns:tts="http://www.w3.org/ns/ttml#styling"><body><p begin="0s" end="3s">
      <span xml:id="host" begin="0s" end="3s" tts:color="red">Hi
        <animate attributeName="tts:color" values="red;blue" keyTimes="0;1" dur="1s" fill="freeze"/>
      </span>
    </p></body></tt>`
	table, err := AnalyzeString(source)
	if err != nil {
		t.Fatal(err)
	}
	run := findRunByText(table, "Hi")
	if run == nil {
		t.Fatal("animated text run missing")
	}

	colorLanes := lanesFor(table, run.ID, "color")
	if len(colorLanes) != 2 {
		t.Fatalf("color lanes = %d (%+v), want linear + freeze", len(colorLanes), colorLanes)
	}
	if colorLanes[0].Interpolation != "linear" || colorLanes[0].Begin != 0 || endOrZeroPtr(colorLanes[0].End) != time.Second {
		t.Fatalf("first lane wrong: %+v", colorLanes[0])
	}
	if colorLanes[1].Interpolation != "freeze" || colorLanes[1].Value != "blue" || endOrZeroPtr(colorLanes[1].End) != 3*time.Second {
		t.Fatalf("freeze lane wrong: %+v", colorLanes[1])
	}
}

func TestAnimationWithoutFreezeRevertsToStaticStyle(t *testing.T) {
	source := `<tt xmlns:tts="http://www.w3.org/ns/ttml#styling"><body><p begin="0s" end="2s">
      <span begin="0s" end="2s" tts:color="red">Hi
        <animate attributeName="tts:color" from="red" to="blue" dur="1s"/>
      </span>
    </p></body></tt>`
	table, err := AnalyzeString(source)
	if err != nil {
		t.Fatal(err)
	}
	run := findRunByText(table, "Hi")
	lane := laneAt(table, run.ID, "color", 1500*time.Millisecond)
	if lane == nil {
		t.Fatal("color style after animation missing")
	}
	if lane.Interpolation != "static" || lane.Value != "red" {
		t.Fatalf("post-animation style = %+v, want static red", lane)
	}
}

func TestHiddenTextIsExcludedFromSliceButRetainedAsRun(t *testing.T) {
	source := `<tt xmlns:tts="http://www.w3.org/ns/ttml#styling"><body><p begin="0s" end="2s">
      <span begin="0s" end="2s">visible</span>
      <span begin="0s" end="2s" tts:visibility="hidden">secret</span>
    </p></body></tt>`
	table, err := AnalyzeString(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(table.Slices) == 0 {
		t.Fatal("expected slices")
	}
	if got := table.Slices[0].Text; got != "visible" {
		t.Fatalf("slice text = %q, want only visible", got)
	}
	run := findRunByText(table, "secret")
	if run == nil {
		t.Fatal("hidden source run should be retained for diagnostics")
	}
}

func lanesFor(table *EventTable, runID, property string) []StyleInterval {
	out := make([]StyleInterval, 0)
	for _, lane := range table.StyleIntervals {
		if lane.RunID == runID && lane.Property == property {
			out = append(out, lane)
		}
	}
	return out
}

func laneAt(table *EventTable, runID, property string, at time.Duration) *StyleInterval {
	for i := range table.StyleIntervals {
		lane := &table.StyleIntervals[i]
		if lane.RunID != runID || lane.Property != property {
			continue
		}
		if at >= lane.Begin && (lane.End == nil || at < *lane.End) {
			return lane
		}
	}
	return nil
}

func endOrZeroPtr(end *time.Duration) time.Duration {
	if end == nil {
		return 0
	}
	return *end
}
