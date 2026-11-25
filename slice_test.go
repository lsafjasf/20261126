package ttmlevents

import (
	"strings"
	"testing"
	"time"
)

const nestedTTML = `<tt xmlns:tts="http://www.w3.org/ns/ttml#styling">
<head><styling>
  <style xml:id="base" tts:color="white" tts:fontSize="16px"/>
  <style xml:id="hot" style="base" tts:color="yellow" tts:fontWeight="bold"/>
</styling></head>
<body><p begin="0s" end="6s" style="base">
  <span xml:id="outer" begin="0s" end="5s" tts:color="red">Outer
    <span xml:id="inner" begin="1s" end="3s" style="hot">Inner</span>
    Tail
  </span>
</p></body></tt>`

func TestNestedSpansProduceAllTextSlices(t *testing.T) {
	table, err := AnalyzeString(nestedTTML)
	if err != nil {
		t.Fatal(err)
	}

	textByBegin := map[time.Duration]string{}
	for _, slice := range table.Slices {
		textByBegin[slice.Begin] = slice.Text
	}

	assertText(t, textByBegin[0], "Outer Tail")
	assertText(t, textByBegin[time.Second], "Outer Inner Tail")
	assertText(t, textByBegin[3*time.Second], "Outer Tail")

	inner := findRunByText(table, "Inner")
	if inner == nil {
		t.Fatal("Inner text run missing")
	}
	if inner.Begin != time.Second || endOrZero(Interval{End: inner.End}) != 3*time.Second {
		t.Fatalf("Inner interval = [%v,%v), want [1s,3s)", inner.Begin, endOrZero(Interval{End: inner.End}))
	}

	var foundBold bool
	for _, style := range table.StyleIntervals {
		if style.RunID == inner.ID && style.Property == "fontWeight" && style.Value == "bold" {
			foundBold = true
		}
	}
	if !foundBold {
		t.Fatalf("inherited/referenced bold style interval missing for %s", inner.ID)
	}
}

func TestSameTimestampHasCausalTotalOrder(t *testing.T) {
	source := `<tt><body><p begin="0s" end="4s">
      <span xml:id="outer" begin="2s" end="3s">
        <span xml:id="inner" begin="0s" end="1s">x</span>
      </span>
      <span xml:id="replacement" begin="3s" end="4s">y</span>
    </p></body></tt>`
	table, err := AnalyzeString(source)
	if err != nil {
		t.Fatal(err)
	}

	group := groupAt(table, 2*time.Second)
	if got := eventTargets(group.Events); !strings.Contains(strings.Join(got, ","), "#outer,#inner") {
		t.Fatalf("begin order = %v, want outer before inner", got)
	}

	group = groupAt(table, 3*time.Second)
	phasesTargets := make([]string, 0, len(group.Events))
	for _, event := range group.Events {
		phasesTargets = append(phasesTargets, event.Phase+":"+event.Target)
	}
	joined := strings.Join(phasesTargets, "|")
	if !strings.Contains(joined, PhaseEnd+":#inner,"+PhaseEnd+":#outer") &&
		!strings.Contains(joined, PhaseEnd+":#inner|"+PhaseEnd+":#outer") {
		t.Fatalf("end order = %v, want inner before outer", phasesTargets)
	}
	if !strings.Contains(joined, PhaseEnd+":#outer|"+PhaseBegin+":#replacement") {
		t.Fatalf("same-time handoff = %v, want outer end before replacement begin", phasesTargets)
	}

	for i := 1; i < len(table.Events); i++ {
		a, b := table.Events[i-1], table.Events[i]
		if a.Sequence >= b.Sequence {
			t.Fatal("event sequence not increasing")
		}
		if a.Time > b.Time {
			t.Fatal("events not time sorted")
		}
	}
}

func TestOutputIsDeterministic(t *testing.T) {
	first, err := AnalyzeString(nestedTTML)
	if err != nil {
		t.Fatal(err)
	}
	second, err := AnalyzeString(nestedTTML)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Events) != len(second.Events) || len(first.Slices) != len(second.Slices) {
		t.Fatal("non-deterministic event/slice count")
	}
	for i := range first.Events {
		if first.Events[i].OrderKey != second.Events[i].OrderKey {
			t.Fatalf("event %d order key differs: %q vs %q", i, first.Events[i].OrderKey, second.Events[i].OrderKey)
		}
	}
}

func assertText(t *testing.T, got, want string) {
	t.Helper()
	if normalizeForCompare(got) != want {
		t.Fatalf("text = %q, want %q", got, want)
	}
}

func normalizeForCompare(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func findRunByText(table *EventTable, text string) *TextRun {
	for i := range table.Runs {
		if normalizeForCompare(table.Runs[i].Text) == text {
			return &table.Runs[i]
		}
	}
	return nil
}

func groupAt(table *EventTable, at time.Duration) EventGroup {
	for _, group := range table.Groups {
		if group.Time == at {
			return group
		}
	}
	return EventGroup{}
}

func eventTargets(events []Event) []string {
	out := make([]string, 0, len(events))
	for _, event := range events {
		out = append(out, event.Target)
	}
	return out
}
