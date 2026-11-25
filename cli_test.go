package sdh

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIWritesArtifacts(t *testing.T) {
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := RunCLI([]string{"testdata/sdh_mixed.srt", "-out", dir, "-base", "mix"},
		os.Stdin, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr.String())
	}
	for _, name := range []string{
		"mix.plain.srt", "mix.sdh.ass", "mix.layers.json",
		"mix.dialogue.srt", "mix.sound.srt", "mix.voiceover.srt",
		"mix.lyric.srt", "mix.speaker.srt", "mix.music.srt",
	} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("expected artifact %s: %v", name, err)
		}
	}
}

func TestCLIPrintPlainFromStdin(t *testing.T) {
	input := strings.NewReader("1\n00:00:01,000 --> 00:00:02,000\n[雷声]\n你好。\n")
	var stdout, stderr bytes.Buffer
	code := RunCLI([]string{"-in", "-", "-out", "", "-print-plain"}, input, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit = %d stderr=%s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "你好。") {
		t.Fatalf("stdout missing dialogue:\n%s", out)
	}
	if strings.Contains(out, "雷声") {
		t.Fatalf("plain output leaked sound description:\n%s", out)
	}
}

func TestCLIPrintReport(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := RunCLI([]string{"-in", "testdata/sdh_mixed.srt", "-out", "", "-print-report"},
		os.Stdin, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit = %d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"complete": true`) {
		t.Fatalf("report should be complete:\n%s", stdout.String())
	}
}

func TestCLIMissingInput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := RunCLI([]string{"-in", filepath.Join(t.TempDir(), "nope.srt")},
		os.Stdin, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
}
