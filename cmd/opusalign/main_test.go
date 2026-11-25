package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunJSONSmoke(t *testing.T) {
	input := strings.NewReader(`{"pre_skip":120,"packets":[{"toc":0}]}`)
	var stdout, stderr bytes.Buffer
	err := run([]string{"-compact"}, input, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run: %v stderr=%s", err, stderr.String())
	}
	output := stdout.String()
	if !strings.Contains(output, `"pre_skip":120`) {
		t.Fatalf("output missing pre-skip: %s", output)
	}
	if !strings.Contains(output, `"start":-120`) {
		t.Fatalf("output missing shifted interval: %s", output)
	}
}

func TestRunRejectsUnknownJSONField(t *testing.T) {
	input := strings.NewReader(`{"preSkip":120}`)
	var stdout, stderr bytes.Buffer
	if err := run(nil, input, &stdout, &stderr); err == nil {
		t.Fatal("unknown JSON field must be rejected")
	}
}
