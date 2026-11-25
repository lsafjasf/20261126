package main

import (
	"errors"
	"testing"
)

func TestCheckExitCodes(t *testing.T) {
	if err := runCheck([]string{"--no-such-flag"}); err == nil {
		t.Fatal("expected flag error")
	}

	err := runCheck([]string{"../../examples/portrait_overlap.vtt"})
	if err != nil {
		t.Fatalf("resolvable example should exit 0, got %v", err)
	}

	err = runCheck([]string{"../../examples/infeasible_stack.vtt"})
	var se silentExit
	if !errors.As(err, &se) || se.code != 1 {
		t.Fatalf("infeasible example should silentExit(1), got %v", err)
	}
}

func TestInterspersedFramesFlags(t *testing.T) {
	dir := t.TempDir()
	err := runFrames([]string{
		"../../examples/portrait_overlap.vtt",
		"--dir", dir,
		"--at", "4.5",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPreviewRuns(t *testing.T) {
	if err := runPreview([]string{
		"../../examples/portrait_overlap.vtt", "--at", "4.5", "--no-color",
	}); err != nil {
		t.Fatal(err)
	}
}
