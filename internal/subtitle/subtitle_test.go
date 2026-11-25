package subtitle

import (
	"testing"
	"time"
)

func TestParseSRT(t *testing.T) {
	raw := []byte("1\n00:00:01,200 --> 00:00:04,000\n<i>斜体</i>与正常\n\n2\n00:00:05,000 --> 00:00:06,500\n第二行\n")
	f, err := Parse("sample.srt", raw)
	if err != nil {
		t.Fatal(err)
	}
	if f.Format != FormatSRT || len(f.Cues) != 2 {
		t.Fatalf("format=%s cues=%d", f.Format, len(f.Cues))
	}
	c := f.Cues[0]
	if c.Index != 1 || c.Start != 1200*time.Millisecond || c.End != 4*time.Second {
		t.Fatalf("bad timing: %+v", c)
	}
	if c.Visible != "斜体与正常" {
		t.Fatalf("visible = %q, tags not stripped", c.Visible)
	}
}

func TestParseVTT(t *testing.T) {
	raw := []byte("WEBVTT\n\n00:01.000 --> 00:02.500\n<c.bold>bold text</c>\n")
	f, err := Parse("sample.vtt", raw)
	if err != nil {
		t.Fatal(err)
	}
	if f.Format != FormatVTT || len(f.Cues) != 1 {
		t.Fatalf("format=%s cues=%d", f.Format, len(f.Cues))
	}
	if f.Cues[0].Visible != "bold text" {
		t.Fatalf("visible = %q", f.Cues[0].Visible)
	}
	if f.Cues[0].Start != time.Second {
		t.Fatalf("start = %v", f.Cues[0].Start)
	}
}

func TestParseASSStylesAndBreaks(t *testing.T) {
	raw := []byte("[Script Info]\nScriptType: v4.00+\n\n[V4+ Styles]\n" +
		"Format: Name, Fontname, Fontsize\nStyle: Default,Nimbus Sans,48\n\n[Events]\n" +
		"Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n" +
		"Dialogue: 0,0:00:01.20,0:00:04.00,Default,,0,0,0,,第一行\\N第二行\n" +
		"Dialogue: 0,0:00:05.00,0:00:06.00,Default,,0,0,0,,{\\fnDroid Sans Fallback}字体覆盖\n")
	f, err := Parse("sample.ass", raw)
	if err != nil {
		t.Fatal(err)
	}
	if f.Format != FormatASS || len(f.Cues) != 2 {
		t.Fatalf("format=%s cues=%d", f.Format, len(f.Cues))
	}
	if f.Cues[0].Index != 1 {
		t.Fatalf("cue index = %d, want 1", f.Cues[0].Index)
	}
	if f.Cues[0].Visible != "第一行\n第二行" {
		t.Fatalf("hard break not converted: %q", f.Cues[0].Visible)
	}
	var sawStyle, sawFileInline bool
	for _, fr := range f.Fonts {
		if fr.Name == "Nimbus Sans" && fr.Source == "style:Default" {
			sawStyle = true
		}
		if fr.Name == "Droid Sans Fallback" && fr.Source == "inline:fn" {
			sawFileInline = true
		}
	}
	if !sawStyle || !sawFileInline {
		t.Fatalf("file-level fonts = %+v", f.Fonts)
	}
	var foundInline bool
	for _, fr := range f.Cues[1].Fonts {
		if fr.Name == "Droid Sans Fallback" && fr.Source == "inline:fn" {
			foundInline = true
		}
	}
	if !foundInline {
		t.Fatalf("inline \\fn not captured: %+v", f.Cues[1].Fonts)
	}
	if f.Cues[1].Visible != "字体覆盖" {
		t.Fatalf("override block not stripped: %q", f.Cues[1].Visible)
	}
}

func TestTimestamp(t *testing.T) {
	got := Timestamp(3_723_450 * time.Millisecond)
	if got != "01:02:03.450" {
		t.Fatalf("timestamp = %q", got)
	}
}

func TestInlineFontFace(t *testing.T) {
	raw := []byte("1\n00:00:01,000 --> 00:00:02,000\n<font face=\"Custom Font\">文本</font>\n")
	f, err := Parse("x.srt", raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Cues[0].Fonts) != 1 || f.Cues[0].Fonts[0].Name != "Custom Font" {
		t.Fatalf("inline font = %+v", f.Cues[0].Fonts)
	}
}
