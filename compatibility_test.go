package ttmlevents

import "testing"

func TestCSSLikeStyleAttributeIsParsed(t *testing.T) {
	table, err := AnalyzeString(`<tt><body><p begin="0s" end="1s">
		<span begin="0s" end="1s" style="color: rgb(255, 0, 0); font-weight: bold">x</span>
	</p></body></tt>`)
	if err != nil {
		t.Fatal(err)
	}
	run := findRunByText(table, "x")
	if run == nil {
		t.Fatal("run missing")
	}
	var color, weight string
	for _, prop := range run.Style {
		switch prop.Name {
		case "color":
			color = prop.Value
		case "fontWeight":
			weight = prop.Value
		}
	}
	if color != "rgb(255, 0, 0)" || weight != "bold" {
		t.Fatalf("css style = color %q weight %q", color, weight)
	}
}

func TestAbsoluteTimingOption(t *testing.T) {
	source := `<tt><body><p begin="10s" end="20s">
		<span id="s" begin="12s" end="14s">x</span>
	</p></body></tt>`
	relative, err := AnalyzeString(source)
	if err != nil {
		t.Fatal(err)
	}
	run := findRunByText(relative, "x")
	if run.TimingVisible {
		t.Fatalf("relative interval should be clipped: %+v", run)
	}

	absolute, err := AnalyzeStringWithOptions(source, TimingOptions{AbsoluteTimes: true})
	if err != nil {
		t.Fatal(err)
	}
	run = findRunByText(absolute, "x")
	if !run.TimingVisible || run.Begin != 12_000_000_000 {
		t.Fatalf("absolute run = visible=%v begin=%v, want visible 12s", run.TimingVisible, run.Begin)
	}
}
