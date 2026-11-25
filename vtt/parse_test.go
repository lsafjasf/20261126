package vtt

import (
	"strings"
	"testing"
)

const minimal = "WEBVTT\n\n1\n00:00:01.000 --> 00:00:02.000\nhello\n"

func TestParseMinimal(t *testing.T) {
	doc, err := ParseString(minimal)
	if err != nil {
		t.Fatalf("ParseString error: %v", err)
	}
	cues := doc.Cues()
	if len(cues) != 1 {
		t.Fatalf("got %d cues, want 1", len(cues))
	}
	cue := cues[0]
	if cue.ID != "1" || cue.Start != 1000 || cue.End != 2000 || cue.Text != "hello" {
		t.Errorf("unexpected cue: %+v", cue)
	}
}

func TestParseBOMAndCRLF(t *testing.T) {
	input := "\ufeffWEBVTT\r\n\r\n00:01.000 --> 00:02.000\r\nline one\r\nline two\r\n"
	doc, err := ParseString(input)
	if err != nil {
		t.Fatalf("ParseString error: %v", err)
	}
	cue := doc.Cues()[0]
	if cue.ID != "" || cue.Start != 1000 || cue.Text != "line one\nline two" {
		t.Errorf("unexpected cue: %+v", cue)
	}
}

func TestParseAnonymousCue(t *testing.T) {
	doc, err := ParseString("WEBVTT\n\n00:01.000 --> 00:02.000\nno id here\n")
	if err != nil {
		t.Fatalf("ParseString error: %v", err)
	}
	if doc.Cues()[0].ID != "" {
		t.Errorf("expected empty cue ID, got %q", doc.Cues()[0].ID)
	}
}

func TestParsePreservesNonCueBlocks(t *testing.T) {
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
		"00:01.000 --> 00:02.000",
		"text",
		"",
	}, "\n")
	doc, err := ParseString(input)
	if err != nil {
		t.Fatalf("ParseString error: %v", err)
	}
	if doc.Header != "- demo" {
		t.Errorf("header = %q", doc.Header)
	}
	var regions, styles, notes int
	for _, block := range doc.Blocks {
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
		t.Errorf("block counts = region:%d style:%d note:%d", regions, styles, notes)
	}
}

func TestParseRejectsInvalidFiles(t *testing.T) {
	inputs := []string{
		"",
		"NOTVTT\n\n00:01.000 --> 00:02.000\nx\n",
		"WEBVTT\n\n1\n00:05.000 --> 00:01.000\nreversed\n",
		"WEBVTT\n\n00:01.000 --> 00:02.000\n",
	}
	for _, input := range inputs {
		if _, err := ParseString(input); err == nil {
			t.Errorf("expected error for input %q", input)
		}
	}
}

func TestParseRepeatedIdentifiers(t *testing.T) {
	input := strings.Join([]string{
		"WEBVTT",
		"",
		"7",
		"00:01.000 --> 00:02.000",
		"first",
		"",
		"7",
		"00:03.000 --> 00:04.000",
		"second",
		"",
	}, "\n")
	doc, err := ParseString(input)
	if err != nil {
		t.Fatalf("ParseString error: %v", err)
	}
	cues := doc.Cues()
	if len(cues) != 2 || cues[0].ID != "7" || cues[1].ID != "7" {
		t.Fatalf("repeated identifiers must both parse, got %+v", cues)
	}
	if cues[0].Identity() == cues[1].Identity() {
		t.Error("different timing/text must produce different identities")
	}
}
