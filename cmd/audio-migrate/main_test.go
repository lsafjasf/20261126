package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunMigratesSample(t *testing.T) {
	input := filepath.Join("..", "..", "testdata", "film.json")
	var stdout, stderr bytes.Buffer
	if code := run([]string{input}, &stdout, &stderr); code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{
		`"type": "dialogue"`,
		`"type": "music"`,
		`"type": "effects"`,
		`"type": "dub"`,
		`"language": "zh-CN"`,
		`"language": "und"`,
	} {
		if !strings.Contains(output, want) {
			t.Errorf("output does not contain %s:\n%s", want, output)
		}
	}
}

func TestRunRejectsInvalidProjectWithoutTouchingOutput(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "input.json")
	output := filepath.Join(dir, "output.json")
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata",
		"invalid-duplicate-default.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(input, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(output, []byte("previous\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"-out", output, input}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("run() code = %d, want 1; stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "duplicate_default") {
		t.Fatalf("stderr does not report duplicate_default: %s", stderr.String())
	}
	content, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "previous\n" {
		t.Fatalf("invalid migration changed output: %q", content)
	}
}
