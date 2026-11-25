package ass

import (
	"math"
	"strings"
	"testing"
	"time"
)

func TestParseReorderedFormats(t *testing.T) {
	const input = `[Script Info]
PlayResX: 1920
PlayResY: 1080
ScaledBorderAndShadow: yes
WrapStyle: 2

[V4+ Styles]
Format: Name, Fontname, Fontsize, MarginV, Alignment, MarginL, MarginR, ScaleX, ScaleY, Outline, Shadow
Style: Caption,Noto Sans,52,64,8,120,130,110,95,3,1

[Events]
Format: Layer, Start, End, Style, MarginL, MarginR, MarginV, Name, Effect, Text
Dialogue: 2,0:00:01.25,0:00:03.50,Caption,10,20,30,Speaker,,Hello, world
`
	script, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if script.Info.PlayResX != 1920 || script.Info.PlayResY != 1080 {
		t.Fatalf("PlayRes = %vx%v", script.Info.PlayResX, script.Info.PlayResY)
	}
	style, ok := script.StyleByName("caption")
	if !ok {
		t.Fatal("style lookup failed")
	}
	if style.MarginV != 64 || style.Alignment != 8 || style.MarginL != 120 ||
		style.MarginR != 130 || style.ScaleX != 110 || style.ScaleY != 95 {
		t.Fatalf("unexpected style: %+v", style)
	}
	if len(script.Events) != 1 {
		t.Fatalf("events = %d", len(script.Events))
	}
	event := script.Events[0]
	if event.Layer != 2 || event.Start != 1250*time.Millisecond ||
		event.End != 3500*time.Millisecond {
		t.Fatalf("unexpected event timing/layer: %+v", event)
	}
	if event.Text != "Hello, world" || event.MarginV != 30 {
		t.Fatalf("unexpected event: %+v", event)
	}
}

func TestParseOverrideVectorClip(t *testing.T) {
	text := `{\an7\fs64\pos(960,540)\clip(m 100 200 l 500 200 500 400 100 400)}你好\Nworld`
	override, warnings := ParseOverride(text)
	if len(warnings) != 0 {
		t.Fatalf("warnings: %v", warnings)
	}
	if override.Alignment != 7 || override.FontSize == nil || *override.FontSize != 64 {
		t.Fatalf("unexpected override: %+v", override)
	}
	if override.Position == nil || override.Position.X != 960 || override.Position.Y != 540 {
		t.Fatalf("position = %+v", override.Position)
	}
	if override.VisibleText != "你好\nworld" {
		t.Fatalf("visible text = %q", override.VisibleText)
	}
	if len(override.Clips) != 1 {
		t.Fatalf("clips = %d", len(override.Clips))
	}
	clip := override.Clips[0]
	want := Bounds{MinX: 100, MinY: 200, MaxX: 500, MaxY: 400}
	if !nearBounds(clip.Bounds, want) {
		t.Fatalf("clip bounds = %+v, want %+v", clip.Bounds, want)
	}
	if !clip.IsVector || clip.Mode != ClipNormal || len(clip.Points) != 4 {
		t.Fatalf("unexpected clip: %+v", clip)
	}
}

func TestParseInverseRectangleAndScaledClip(t *testing.T) {
	override, warnings := ParseOverride(`{\iclip(10,20,300,400)}mask`)
	if len(warnings) != 0 {
		t.Fatalf("warnings: %v", warnings)
	}
	clip := override.Clips[0]
	if clip.Mode != ClipInverse || clip.IsVector {
		t.Fatalf("clip mode/type = %s/%v", clip.Mode, clip.IsVector)
	}
	if !nearBounds(clip.Bounds, Bounds{MinX: 10, MinY: 20, MaxX: 300, MaxY: 400}) {
		t.Fatalf("bounds = %+v", clip.Bounds)
	}

	override, warnings = ParseOverride(`{\clip(2,m 1 2 l 4 6)}scaled`)
	if len(warnings) != 0 {
		t.Fatalf("warnings: %v", warnings)
	}
	if !nearBounds(override.Clips[0].Bounds, Bounds{MinX: 2, MinY: 4, MaxX: 8, MaxY: 12}) {
		t.Fatalf("scaled bounds = %+v", override.Clips[0].Bounds)
	}
}

func TestParseTime(t *testing.T) {
	got, err := ParseTime("1:02:03.45")
	if err != nil {
		t.Fatal(err)
	}
	want := time.Hour + 2*time.Minute + 3*time.Second + 450*time.Millisecond
	if got != want {
		t.Fatalf("ParseTime = %v, want %v", got, want)
	}
}

func nearBounds(a, b Bounds) bool {
	return math.Abs(a.MinX-b.MinX) < 1e-6 &&
		math.Abs(a.MinY-b.MinY) < 1e-6 &&
		math.Abs(a.MaxX-b.MaxX) < 1e-6 &&
		math.Abs(a.MaxY-b.MaxY) < 1e-6
}
