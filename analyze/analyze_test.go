package analyze

import (
	"math"
	"strings"
	"testing"

	"assclip/ass"
	"assclip/layout"
)

const fixture = `[Script Info]
PlayResX: 1920
PlayResY: 1080
ScaledBorderAndShadow: yes

[V4+ Styles]
Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding
Style: Default,Arial,48,&H00FFFFFF,&H000000FF,&H00000000,&H00000000,0,0,0,0,100,100,0,0,1,2,0,2,100,100,50,1

[Events]
Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text
Dialogue: 0,0:00:00.00,0:00:02.00,Default,,0,0,0,,{\clip(m 0 600 l 1920 600 1920 1060 0 1060)}Bottom caption
Dialogue: 0,0:00:02.00,0:00:04.00,Default,,0,0,0,,{\clip(0,0,1920,400)}Cropped caption
Dialogue: 0,0:00:04.00,0:00:06.00,Default,,0,0,0,,{\an7\pos(10,10)}Header
Dialogue: 0,0:00:06.00,0:00:08.00,Default,,0,0,0,,{\iclip(0,0,1920,1080)}Inverse
`

func TestAnalyzePortraitFit(t *testing.T) {
	script := parseFixture(t)
	canvas, err := layout.NewCanvas(1920, 1080, 1080, 1920, layout.Fit)
	if err != nil {
		t.Fatal(err)
	}
	report := Analyze(script, Config{Canvas: canvas})

	if report.Summary.Cues != 4 || report.Summary.CuesWithClip != 3 {
		t.Fatalf("summary = %+v", report.Summary)
	}
	assertNear(t, "scale", report.Target.ScaleX, 0.5625)
	assertNear(t, "offset y", report.Target.OffsetY, 656.25)

	good := report.Cues[0]
	assertRectNear(t, "good clip", good.Clip.Bounds, layout.Rect{
		MinX: 0, MinY: 993.75, MaxX: 1080, MaxY: 1252.5,
	})
	if hasConflict(good, "text_cropped_by_clip") {
		t.Fatalf("good cue unexpectedly cropped: %+v", good.Conflicts)
	}
	assertRectNear(t, "safe area", good.SafeArea.Rect, layout.Rect{
		MinX: 56.25, MinY: 656.25, MaxX: 1023.75, MaxY: 1235.625,
	})

	cropped := report.Cues[1]
	if !hasConflict(cropped, "text_cropped_by_clip") {
		t.Fatalf("cropped cue conflicts = %+v", cropped.Conflicts)
	}

	header := report.Cues[2]
	if !hasConflict(header, "text_outside_safe_area") {
		t.Fatalf("header conflicts = %+v", header.Conflicts)
	}

	inverse := report.Cues[3]
	if !hasConflict(inverse, "text_hidden_by_inverse_clip") {
		t.Fatalf("inverse conflicts = %+v", inverse.Conflicts)
	}
}

func TestAnalyzeUltrawideFill(t *testing.T) {
	script := parseFixture(t)
	canvas, err := layout.NewCanvas(1920, 1080, 3440, 1440, layout.Fill)
	if err != nil {
		t.Fatal(err)
	}
	report := Analyze(script, Config{Canvas: canvas})

	assertNear(t, "scale", report.Target.ScaleX, 1.7916666667)
	assertNear(t, "offset y", report.Target.OffsetY, -247.5)
	first := report.Cues[0]
	assertRectNear(t, "ultrawide clip", first.Clip.Bounds, layout.Rect{
		MinX: 0, MinY: 827.5, MaxX: 3440, MaxY: 1651.6666667,
	})
	if !hasConflict(first, "clip_outside_frame") ||
		!hasConflict(first, "text_outside_frame") {
		t.Fatalf("ultrawide conflicts = %+v", first.Conflicts)
	}
	if first.Clip.FrameOverflow.Bottom <= 0 {
		t.Fatalf("expected bottom overflow, got %+v", first.Clip.FrameOverflow)
	}
}

func TestAnalyzeSubjectAndInverseVisibleArea(t *testing.T) {
	script := parseFixture(t)
	portrait, _ := layout.NewCanvas(1920, 1080, 1080, 1920, layout.Fit)
	report := Analyze(script, Config{
		Canvas:   portrait,
		Subjects: []layout.Rect{{MinX: 0, MinY: 650, MaxX: 300, MaxY: 720}},
	})
	if !hasConflict(report.Cues[2], "text_visible_over_subject") {
		t.Fatalf("expected subject collision: %+v", report.Cues[2].Conflicts)
	}

	ultrawide, _ := layout.NewCanvas(1920, 1080, 3440, 1440, layout.Fill)
	report = Analyze(script, Config{
		Canvas:   ultrawide,
		Subjects: []layout.Rect{{MinX: 0, MinY: 0, MaxX: 3440, MaxY: 1440}},
	})
	if hasConflict(report.Cues[3], "text_visible_over_subject") {
		t.Fatalf("inverse clip should hide text over the subject: %+v", report.Cues[3].Conflicts)
	}
}

func TestAnalyzeStretchMargins(t *testing.T) {
	script := parseFixture(t)
	canvas, err := layout.NewCanvas(1920, 1080, 1080, 1920, layout.Stretch)
	if err != nil {
		t.Fatal(err)
	}
	report := Analyze(script, Config{Canvas: canvas})
	assertNear(t, "stretch x", report.Target.ScaleX, 0.5625)
	assertNear(t, "stretch y", report.Target.ScaleY, 1.7777777778)
	assertNear(t, "left margin", report.Cues[0].MarginsPixels.Left, 56.25)
	assertNear(t, "vertical margin", report.Cues[0].MarginsPixels.Vertical, 88.8888888889)
}

func parseFixture(t *testing.T) *ass.Script {
	t.Helper()
	script, err := ass.Parse(strings.NewReader(fixture))
	if err != nil {
		t.Fatal(err)
	}
	return script
}

func hasConflict(cue CueReport, code string) bool {
	for _, conflict := range cue.Conflicts {
		if conflict.Code == code {
			return true
		}
	}
	return false
}

func assertNear(t *testing.T, label string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-6 {
		t.Fatalf("%s = %.10f, want %.10f", label, got, want)
	}
}

func assertRectNear(t *testing.T, label string, got, want layout.Rect) {
	t.Helper()
	if math.Abs(got.MinX-want.MinX) > 1e-6 ||
		math.Abs(got.MinY-want.MinY) > 1e-6 ||
		math.Abs(got.MaxX-want.MaxX) > 1e-6 ||
		math.Abs(got.MaxY-want.MaxY) > 1e-6 {
		t.Fatalf("%s = %+v, want %+v", label, got, want)
	}
}
