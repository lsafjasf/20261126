package vttlayout

import (
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func assertWellFormedXML(t *testing.T, svg string) {
	t.Helper()
	dec := xml.NewDecoder(strings.NewReader(svg))
	for {
		if _, err := dec.Token(); err != nil {
			if err == io.EOF {
				return
			}
			t.Fatalf("SVG not well formed: %v\n%s", err, svg[:min(400, len(svg))])
		}
	}
}

func TestSVGFramesWellFormed(t *testing.T) {
	doc, plan := analyzeExample(t, ExampleOverlapVTT)
	times := KeyFrameTimes(doc)
	if len(times) == 0 {
		t.Fatal("no key frames")
	}
	for _, at := range times {
		for _, mode := range []FrameMode{FrameBefore, FrameAfter} {
			svg := RenderSVGFrame(plan, at, mode)
			if !strings.HasPrefix(svg, "<svg") || !strings.HasSuffix(svg, "</svg>") {
				t.Fatal("svg wrapper missing")
			}
			assertWellFormedXML(t, svg)
		}
	}
	overview := RenderSVGFrame(plan, -1, FrameAfter)
	assertWellFormedXML(t, overview)
	if !strings.Contains(overview, "主持人") {
		t.Fatal("overview should contain cue text")
	}
}

func TestWriteFrames(t *testing.T) {
	doc, plan := analyzeExample(t, ExampleOverlapVTT)
	dir := t.TempDir()
	times := []Milliseconds{2500, 4500}
	written, index, err := WriteFrames(dir, doc, plan, times)
	if err != nil {
		t.Fatal(err)
	}
	if len(written) != 4 {
		t.Fatalf("want 4 frames, got %d", len(written))
	}
	if _, err := os.Stat(index); err != nil {
		t.Fatalf("index missing: %v", err)
	}
	svgs, _ := filepath.Glob(filepath.Join(dir, "*.svg"))
	if len(svgs) != 4 {
		t.Fatalf("want 4 svg files, got %d", len(svgs))
	}
}

func TestTerminalFrame(t *testing.T) {
	_, plan := analyzeExample(t, ExampleOverlapVTT)
	frame := RenderTerminalFrame(plan, 4500, FrameAfter, 80, false)
	if !strings.Contains(frame, "┌") || !strings.Contains(frame, "主持人") {
		t.Fatalf("terminal frame missing border/text:\n%s", frame)
	}
	if strings.Contains(frame, "\x1b[") {
		t.Fatal("no-color frame should not contain ANSI escapes")
	}
	color := RenderTerminalFrame(plan, 4500, FrameBefore, 80, true)
	if !strings.Contains(color, "\x1b[") {
		t.Fatal("color frame should contain ANSI escapes")
	}
}
