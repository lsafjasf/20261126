package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mediaanchor/anchor"
)

func runCLI(t *testing.T, args ...string) string {
	t.Helper()
	dir := t.TempDir()
	prevArgs := os.Args
	defer func() { os.Args = prevArgs }()

	// Redirect stdout into a pipe by swapping the process stdout is too
	// invasive; instead capture via a helper that writes into a buffer.
	out, err := capture(func() error {
		os.Args = append([]string{"mediaanchor"}, args...)
		return dispatch(args)
	})
	if err != nil {
		t.Fatalf("cli %v: %v", args, err)
	}
	_ = dir
	return out
}

func TestCLIDemoAndChecksumStability(t *testing.T) {
	dir := t.TempDir()
	if out := runCLI(t, "demo", dir); !strings.Contains(out, "reopen stability: OK") {
		t.Fatalf("demo output missing stability confirmation:\n%s", out)
	}

	resolved := filepath.Join(dir, "demo-resolved.json")
	p, err := anchor.Load(resolved)
	if err != nil {
		t.Fatal(err)
	}
	result, err := p.Map()
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Unresolved) != 0 || len(result.Stale) != 0 {
		t.Fatalf("resolved demo must have no unresolved/stale items: %+v", result)
	}

	checksumOut := strings.TrimSpace(runCLI(t, "checksum", resolved))
	want, err := p.Checksum()
	if err != nil {
		t.Fatal(err)
	}
	if checksumOut != want {
		t.Fatalf("checksum command = %q, want %q", checksumOut, want)
	}
}

func TestCLIResolveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	runCLI(t, "demo", dir)
	resolved := filepath.Join(dir, "demo-resolved.json")

	key := anchor.PieceKey("SUB1", anchor.Anchor{
		MediaID: "media-A", Start: 7_000_000, End: 9_000_000,
	})

	// Drop the existing segment decision, which must make SUB1 ambiguous again.
	if out := runCLI(t, "unresolve", "--key", key, resolved); !strings.Contains(out, "remaining unresolved: 1") {
		t.Fatalf("unresolve output:\n%s", out)
	}

	out := runCLI(t, "resolve", "--key", key,
		"--segment", "seg-4", "--reason", "prefer flashback in review", resolved)
	if !strings.Contains(out, "decision recorded") || !strings.Contains(out, "remaining unresolved: 0") {
		t.Fatalf("resolve output:\n%s", out)
	}

	p, err := anchor.Load(resolved)
	if err != nil {
		t.Fatal(err)
	}
	if p.Decisions[key].SegmentID != "seg-4" {
		t.Fatalf("decision not persisted: %+v", p.Decisions[key])
	}
}

func TestCLIAnchorForLegacyEvent(t *testing.T) {
	dir := t.TempDir()
	runCLI(t, "demo", dir)
	initial := filepath.Join(dir, "demo-initial.json")

	out := runCLI(t, "anchor",
		"--event", "LEGACY1", "--media", "media-A",
		"--start", "12s", "--end", "14s", "--reason", "cli recovery", initial)
	if !strings.Contains(out, "anchor decision recorded") {
		t.Fatalf("anchor output:\n%s", out)
	}
}
