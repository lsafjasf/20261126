package main

import (
	"path/filepath"
	"testing"
)

func TestCLIInspectCheckAndFix(t *testing.T) {
	input := filepath.Join("..", "..", "testdata", "g711-stereo-bad-fact.wav")

	if err := runInspect([]string{"-json", input}); err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if err := runCheck([]string{input}); err == nil {
		t.Fatal("check accepted a file with a known fact mismatch")
	}

	output := filepath.Join(t.TempDir(), "fixed.wav")
	if err := runFix([]string{"-o", output, input}); err != nil {
		t.Fatalf("fix: %v", err)
	}
	if err := runCheck([]string{output}); err != nil {
		t.Fatalf("check repaired file: %v", err)
	}
}
