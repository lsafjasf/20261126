package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/lsafjasf/20261126/audioedit"
)

const sampleProject = `{
  "params": {"sample_rate": 48000},
  "sources": [{"id": "a", "format": "aac", "sample_rate": 48000,
    "codec_delay": 2112, "edit": {"media_end": 48000}}],
  "clips": [{"id": "c", "source_id": "a"}]
}`

func TestRunWritesPositionTable(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"-compact"}, strings.NewReader(sampleProject), &out); err != nil {
		t.Fatalf("run: %v", err)
	}
	var table audioedit.Table
	if err := json.Unmarshal(out.Bytes(), &table); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if len(table.Segments) != 1 {
		t.Fatalf("segments = %d, want 1", len(table.Segments))
	}
	if table.Segments[0].PrerollSamples != 2112 {
		t.Fatalf("preroll = %d, want 2112", table.Segments[0].PrerollSamples)
	}
}

func TestRunRejectsUnknownProjectField(t *testing.T) {
	input := strings.Replace(sampleProject, `"format": "aac"`, `"format": "aac", "typo": 1`, 1)
	err := run(nil, strings.NewReader(input), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("error = %v, want unknown-field error", err)
	}
}
