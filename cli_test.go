package safereflow

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCLIEndToEndApply(t *testing.T) {
	dir := t.TempDir()
	reportPath := filepath.Join(dir, "report.json")
	cuesPath := filepath.Join(dir, "cues.out.json")
	srtPath := filepath.Join(dir, "out.srt")
	applyPath := filepath.Join(dir, "apply.json")

	var stdout, stderr bytes.Buffer
	code := RunCLI([]string{
		"-safe", "testdata/safe_areas.json",
		"-cues", "testdata/cues.json",
		"-out", reportPath,
		"-apply",
		"-write-cues", cuesPath,
		"-write-srt", srtPath,
		"-apply-report", applyPath,
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit=%d\nstderr=%s", code, stderr.String())
	}

	var rep Report
	decodeFile(t, reportPath, &rep)
	if rep.Summary.Cues != 4 || rep.Summary.Violations != 2 {
		t.Fatalf("summary = %+v", rep.Summary)
	}
	if rep.Summary.Suspected != 1 || rep.Summary.Indeterminate != 1 {
		t.Fatalf("summary status counts wrong: %+v", rep.Summary)
	}

	var ar ApplyReport
	decodeFile(t, applyPath, &ar)
	if len(ar.Applied) != 2 {
		t.Fatalf("applied = %+v", ar.Applied)
	}
	if len(ar.Blocked) != 1 || ar.Blocked[0].Reason != "low_confidence_move_not_automatic" {
		t.Fatalf("blocked = %+v", ar.Blocked)
	}

	doc, err := LoadCueFile(cuesPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range doc.Cues {
		switch c.ID {
		case "c-lowconf":
			if c.Box == nil || c.Box.X != 100 {
				t.Fatalf("low confidence cue must be unmoved, box=%+v", c.Box)
			}
		case "c-gap":
			if c.Box == nil || c.Box.X != 140 {
				t.Fatalf("coverage-gap cue must be untouched, box=%+v", c.Box)
			}
		case "c-closeup":
			if c.Box == nil || !near(c.Box.X, 420) || !near(c.Box.Y, 820) {
				t.Fatalf("closeup migrated box = %+v", c.Box)
			}
		}
	}

	if data, err := os.ReadFile(srtPath); err != nil || len(data) == 0 {
		t.Fatalf("srt output missing: %v", err)
	}
}

func TestCLISRTInput(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "report.json")
	var stdout, stderr bytes.Buffer
	code := RunCLI([]string{
		"-safe", "testdata/safe_areas.json",
		"-srt", "testdata/subtitles.srt",
		"-canvas", "1920x1080",
		"-out", out,
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	var rep Report
	decodeFile(t, out, &rep)
	if rep.Summary.Cues != 3 {
		t.Fatalf("expected 3 srt cues, summary=%+v", rep.Summary)
	}
}

func TestCLIExplicitLowConfidenceMoveRejected(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "report.json")
	var stdout, stderr bytes.Buffer
	code := RunCLI([]string{
		"-safe", "testdata/safe_areas.json",
		"-cues", "testdata/cues.json",
		"-out", out,
		"-plan", "c-lowconf-migrate",
	}, &stdout, &stderr)
	if code != 3 {
		t.Fatalf("expected exit 3 blocked, got %d: %s", code, stderr.String())
	}
}

func decodeFile(t *testing.T, path string, v any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatal(err)
	}
}
