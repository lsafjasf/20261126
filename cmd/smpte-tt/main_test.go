package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunRendersSample(t *testing.T) {
	out := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"-in", "../../testdata/sample.ttml",
		"-out", out,
		"-times", "0s,3.5s",
		"-strict",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "visible leaks: 0") {
		t.Fatalf("unexpected summary: %s", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(out, "manifest.json")); err != nil {
		t.Fatal(err)
	}
}

func TestRunHelpAndBadTime(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-h"}, &stdout, &stderr); code != 0 {
		t.Fatalf("help exit = %d", code)
	}
	stdout.Reset()
	stderr.Reset()
	code := run([]string{
		"-in", "../../testdata/sample.ttml",
		"-out", t.TempDir(),
		"-times", "not-a-time",
	}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("bad time exit = %d, stderr=%s", code, stderr.String())
	}
}
