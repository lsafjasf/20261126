package main

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"assclip/analyze"
	"assclip/layout"
)

func TestParsePlayRes(t *testing.T) {
	w, h, err := parsePlayRes("1920x1080")
	if err != nil {
		t.Fatal(err)
	}
	if w != 1920 || h != 1080 {
		t.Fatalf("play-res = %vx%v", w, h)
	}
	if _, _, err := parsePlayRes("1920"); err == nil {
		t.Fatal("expected malformed play-res error")
	}
}

func TestSubjectFlag(t *testing.T) {
	var subjects rectFlag
	if err := subjects.Set("1200,300,1000,700"); err != nil {
		t.Fatal(err)
	}
	if len(subjects) != 1 {
		t.Fatalf("subjects = %d", len(subjects))
	}
	got := subjects[0]
	if got.MinX != 1200 || got.MinY != 300 || math.Abs(got.MaxX-2200) > 1e-9 ||
		math.Abs(got.MaxY-1000) > 1e-9 {
		t.Fatalf("subject = %+v", got)
	}
}

func TestWriteJSONReport(t *testing.T) {
	canvas, err := layout.NewCanvas(1920, 1080, 1080, 1920, layout.Fit)
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	report := analyze.Report{
		Script: analyze.ScriptReport{PlayResX: 1920, PlayResY: 1080},
		Target: canvas,
		Frame:  canvas.Frame(),
		Cues:   []analyze.CueReport{{Index: 1}},
	}
	if err := writeReport(&out, report, "json"); err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(out.String()), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out.String())
	}
	if decoded["target"] == nil {
		t.Fatal("JSON has no target")
	}
}
