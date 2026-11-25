package vtt

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSerializeRoundTrip(t *testing.T) {
	input := strings.Join([]string{
		"WEBVTT - demo",
		"",
		"REGION",
		"id:top",
		"",
		"STYLE",
		"::cue { color: red; }",
		"",
		"NOTE keep me",
		"",
		"1",
		"00:00:01.000 --> 00:00:02.000 line:50%",
		"first line",
		"second line",
		"",
		"1",
		"00:00:03.000 --> 00:00:04.000",
		"repeated id, different segment",
		"",
	}, "\n")

	doc, err := ParseString(input)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	data, err := doc.Serialize(WriteOptions{})
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}

	reparsed, err := Parse(data)
	if err != nil {
		t.Fatalf("reparse exported file: %v\n%s", err, data)
	}
	if len(reparsed.Cues()) != 2 {
		t.Fatalf("reparsed cue count = %d", len(reparsed.Cues()))
	}
	var regions, styles, notes int
	for _, block := range reparsed.Blocks {
		switch block.(type) {
		case *Region:
			regions++
		case *Style:
			styles++
		case *Note:
			notes++
		}
	}
	if regions != 1 || styles != 1 || notes != 1 {
		t.Errorf("metadata blocks lost: region:%d style:%d note:%d", regions, styles, notes)
	}
	if reparsed.Cues()[0].Settings != "line:50%" {
		t.Errorf("settings not preserved: %q", reparsed.Cues()[0].Settings)
	}
}

func TestSerializeCRLFAndBOM(t *testing.T) {
	doc, err := ParseString(strings.Join([]string{
		"WEBVTT",
		"",
		"NOTE first line",
		"second note line",
		"",
		"00:01.000 --> 00:02.000",
		"line one",
		"line two",
		"",
	}, "\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	data, err := doc.Serialize(WriteOptions{WithBOM: true, CRLF: true})
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}
	if !strings.HasPrefix(string(data), bom) {
		t.Error("BOM missing")
	}
	text := string(data)
	if strings.Count(text, "\n") != strings.Count(text, "\r\n") {
		t.Errorf("found bare LF in CRLF output:\n%q", data)
	}
	if _, err := Parse(data); err != nil {
		t.Errorf("exported CRLF/BOM file not reparseable: %v", err)
	}
	if !strings.Contains(text, "first line\r\nsecond note line") ||
		!strings.Contains(text, "line one\r\nline two") {
		t.Errorf("multiline bodies not converted to CRLF:\n%q", data)
	}
}

func TestValidateRejectsBrokenCues(t *testing.T) {
	if err := validateCue(cueWith("1", 2000, 1000, "reversed")); err == nil {
		t.Error("expected validation error for reversed timing")
	}

	if err := validateCue(cueWith("bad --> id", 1000, 2000, "x")); err == nil {
		t.Error("expected validation error for identifier containing -->")
	}
}

func TestDeduplicatedExportAcceptsRepeatedIDs(t *testing.T) {
	doc := &Document{Blocks: []Block{
		cueWith("9", 1000, 2000, "a"),
		cueWith("9", 1000, 2000, "a"),
		cueWith("9", 3000, 4000, "b"),
	}}
	report := doc.Deduplicate()
	if report.Removed != 1 || len(doc.Cues()) != 2 {
		t.Fatalf("unexpected dedup result: %+v", report)
	}
	data, err := doc.Serialize(WriteOptions{})
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}
	if count := strings.Count(string(data), "\n9\n"); count != 2 {
		t.Errorf("export should contain both id-9 cues, got %d occurrences:\n%s", count, data)
	}
	if _, err := Parse(data); err != nil {
		t.Fatalf("export not reparseable: %v\n%s", err, data)
	}
}

func TestExportAcceptedByFFmpeg(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not available; skipping external parser check")
	}

	doc, err := ParseString(strings.Join([]string{
		"WEBVTT",
		"",
		"1",
		"00:00:01.000 --> 00:00:02.000",
		"first",
		"",
		"1",
		"00:00:03.000 --> 00:00:04.000",
		"second",
		"",
	}, "\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	data, err := doc.Serialize(WriteOptions{})
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "export.vtt")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error",
		"-i", path, "-f", "webvtt", "-")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ffmpeg rejected export: %v\n%s\n%s", err, output, data)
	}
	if !strings.Contains(string(output), "first") || !strings.Contains(string(output), "second") {
		t.Errorf("ffmpeg output missing cue text:\n%s", output)
	}
}
