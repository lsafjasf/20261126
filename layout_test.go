package cuemap

import (
	"testing"
)

func TestParseHTMLRubyAttachesToRuneInterval(t *testing.T) {
	raw := `<ruby>東<rp>(</rp><rt>とう</rt><rp>)</rp></ruby>京`
	cue := ParseCue("jp-1", raw)

	if cue.Text != "東京" {
		t.Fatalf("Text = %q, want 東京", cue.Text)
	}
	if cue.Raw != raw {
		t.Fatalf("Raw was not preserved")
	}
	if len(cue.Ruby) != 1 {
		t.Fatalf("Ruby count = %d, want 1", len(cue.Ruby))
	}
	span := cue.Ruby[0]
	if span.StartRune != 0 || span.EndRune != 1 || span.Text != "とう" {
		t.Fatalf("span = %+v, want [0,1) とう", span)
	}
}

func TestParseAozoraRuby(t *testing.T) {
	cue := ParseCue("jp-2", `｜東京《とうきょう》`)
	if cue.Text != "東京" {
		t.Fatalf("Text = %q, want 東京", cue.Text)
	}
	if len(cue.Ruby) != 1 || cue.Ruby[0].StartRune != 0 || cue.Ruby[0].EndRune != 2 {
		t.Fatalf("Ruby = %+v, want [0,2)", cue.Ruby)
	}
}

func TestVerticalRubyUsesSeparateLaneAndLinePitch(t *testing.T) {
	metrics := DefaultMetrics()
	cue := Cue{
		ID:   "vertical-ruby",
		Text: "東京",
		Raw:  "vertical-ruby-raw",
		Ruby: []RubySpan{
			{StartRune: 0, EndRune: 1, Text: "とう"},
			{StartRune: 1, EndRune: 2, Text: "きょう"},
		},
	}
	result := Typeset(cue, Options{Mode: ModeVertical, Metrics: metrics})
	if !result.OK {
		t.Fatalf("Typeset fallback: %+v", result.Fallback)
	}
	if got := result.Lines[0].Bounds.Width; got < metrics.BaseSize+metrics.RubyGap+metrics.RubySize {
		t.Fatalf("line width = %d, want ruby lane >= %d", got, metrics.BaseSize+metrics.RubyGap+metrics.RubySize)
	}
	if FindCollision(result) != nil {
		t.Fatalf("unexpected collision: %v", FindCollision(result))
	}
}

func TestVerticalLongAdjacentRubyExpandsPitch(t *testing.T) {
	cue := Cue{
		ID:   "long-ruby",
		Text: "漢字",
		Ruby: []RubySpan{
			{StartRune: 0, EndRune: 1, Text: "ああああ"},
			{StartRune: 1, EndRune: 2, Text: "いいいい"},
		},
	}
	result := Typeset(cue, Options{Mode: ModeVertical})
	if !result.OK {
		t.Fatalf("Typeset fallback: %+v", result.Fallback)
	}
	if pitch := result.Lines[0].BasePitch; pitch < 48 {
		t.Fatalf("BasePitch = %d, want >= 48 to separate four-rune ruby blocks", pitch)
	}
}

func TestVerticalColumnsReserveRubyGap(t *testing.T) {
	metrics := DefaultMetrics()
	cue := Cue{
		ID:   "columns",
		Text: "中文\n東",
		Ruby: []RubySpan{{StartRune: 3, EndRune: 4, Text: "ひがし"}},
	}
	result := Typeset(cue, Options{Mode: ModeVertical, Metrics: metrics})
	if !result.OK {
		t.Fatalf("Typeset fallback: %+v", result.Fallback)
	}
	if len(result.Lines) != 2 {
		t.Fatalf("lines = %d, want 2 columns", len(result.Lines))
	}
	if result.Lines[0].ColumnPitch < metrics.BaseSize+metrics.RubyGap+metrics.RubySize {
		t.Fatalf("ColumnPitch = %d, want ruby lane reservation", result.Lines[0].ColumnPitch)
	}
	if collision := FindCollision(result); collision != nil {
		t.Fatalf("column/ruby collision: %v", collision)
	}

	leftColumn := result.Lines[1]
	rightColumn := result.Lines[0]
	leftRubyRight := 0
	for _, cell := range leftColumn.RubyCells {
		if edge := cell.Rect.Right(); edge > leftRubyRight {
			leftRubyRight = edge
		}
	}
	rightBaseLeft := rightColumn.BaseCells[0].Rect.X
	if rightBaseLeft < leftRubyRight {
		t.Fatalf("right base x=%d overlaps left ruby right=%d", rightBaseLeft, leftRubyRight)
	}
}

func TestHorizontalLayoutHasNoCollision(t *testing.T) {
	cue := Cue{
		ID:   "horizontal",
		Text: "東京",
		Ruby: []RubySpan{{StartRune: 0, EndRune: 2, Text: "とうきょう"}},
	}
	result := Typeset(cue, Options{Mode: ModeHorizontal})
	if !result.OK {
		t.Fatalf("Typeset fallback: %+v", result.Fallback)
	}
	if FindCollision(result) != nil {
		t.Fatalf("unexpected collision: %v", FindCollision(result))
	}
}

func TestTypesetFallbackPointsToOriginalCue(t *testing.T) {
	raw := "原始 cue payload: <bad>"
	cue := Cue{
		ID:   "cue-42",
		Text: "abc",
		Raw:  raw,
		Ruby: []RubySpan{
			{StartRune: 0, EndRune: 1, Text: "x"},
			{StartRune: 0, EndRune: 1, Text: "y"},
		},
	}
	result := Typeset(cue, Options{Mode: ModeVertical})
	if result.OK || result.Fallback == nil {
		t.Fatalf("result = %+v, want fallback", result)
	}
	if result.Fallback.Reason != FallbackOverlapRuby {
		t.Fatalf("reason = %q, want %q", result.Fallback.Reason, FallbackOverlapRuby)
	}
	if result.Fallback.CueID != "cue-42" || result.Fallback.CueRef != "cue:cue-42" {
		t.Fatalf("fallback ref = %+v", result.Fallback)
	}
	if result.Fallback.OriginalText != raw || result.Fallback.Cue.Raw != raw {
		t.Fatalf("fallback does not preserve original cue: %+v", result.Fallback)
	}
}

func TestTypesetFallbackOnBoundsExceeded(t *testing.T) {
	cue := Cue{ID: "too-tall", Text: "中文", Raw: "RAW-CUE"}
	metrics := DefaultMetrics()
	metrics.MaxHeight = 1
	result := Typeset(cue, Options{Mode: ModeVertical, Metrics: metrics})
	if result.OK || result.Fallback == nil {
		t.Fatalf("result = %+v, want bounds fallback", result)
	}
	if result.Fallback.Reason != FallbackBounds {
		t.Fatalf("reason = %q, want %q", result.Fallback.Reason, FallbackBounds)
	}
	if result.Fallback.OriginalText != "RAW-CUE" {
		t.Fatalf("OriginalText = %q, want RAW-CUE", result.Fallback.OriginalText)
	}
}
