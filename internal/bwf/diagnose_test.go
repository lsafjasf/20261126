package bwf

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestAnalyzeConflictFixture(t *testing.T) {
	audio := filepath.Join("..", "..", "testdata", "timeref-conflict.bwf")
	project := filepath.Join("..", "..", "testdata", "timeref-conflict.project.json")
	r, err := Analyze(audio, project)
	if err != nil {
		t.Fatal(err)
	}
	if r.Fmt == nil || r.Fmt.SampleRate != 48000 || r.Fmt.BlockAlign != 2 {
		t.Fatalf("unexpected fmt: %+v", r.Fmt)
	}
	if r.Audio.Frames != 240000 {
		t.Fatalf("frames=%d, want 240000", r.Audio.Frames)
	}
	if r.Audio.DurationSeconds != 5 {
		t.Fatalf("duration=%f, want 5", r.Audio.DurationSeconds)
	}
	if r.BEXT == nil || r.BEXT.TimeReference != 1728000384 {
		t.Fatalf("unexpected bext: %+v", r.BEXT)
	}
	if r.IXML == nil || r.IXML.DirectTimeReference == nil || *r.IXML.DirectTimeReference != 1728000000 {
		t.Fatalf("unexpected ixml: %+v", r.IXML)
	}
	if len(r.Starts) != 3 {
		t.Fatalf("starts=%d, want 3: %+v", len(r.Starts), r.Starts)
	}
	found := findComparison(r.Comparisons, "bext.timeReference", "iXML.BWF_TIME_REFERENCE")
	if found == nil || found.DeltaSamples != 384 {
		t.Fatalf("bext/iXML comparison=%+v", found)
	}
	if !hasFinding(r.Findings, "project_duration_diff") {
		t.Fatal("missing project_duration_diff finding")
	}
	if hasFinding(r.Findings, "ixml_error") {
		t.Fatal("iXML should parse successfully")
	}
	if !hasHypothesis(r.Hypotheses, "同一个 384 样本") {
		t.Fatalf("missing shared-offset hypothesis: %+v", r.Hypotheses)
	}
	if len(r.Undecidable) < 3 {
		t.Fatalf("expected unresolved provenance questions, got %d", len(r.Undecidable))
	}
}

func TestAnalyzeRejectsNonWAV(t *testing.T) {
	path := filepath.Join("..", "..", "README.md")
	if _, err := Analyze(path, ""); err == nil {
		t.Fatal("non-RIFF input should fail")
	}
}

func findComparison(cs []Comparison, a, b string) *Comparison {
	for i := range cs {
		if (cs[i].A == a && cs[i].B == b) || (cs[i].A == b && cs[i].B == a) {
			return &cs[i]
		}
	}
	return nil
}

func hasFinding(fs []Finding, code string) bool {
	for _, f := range fs {
		if f.Code == code {
			return true
		}
	}
	return false
}

func hasHypothesis(hs []Hypothesis, needle string) bool {
	for _, h := range hs {
		if strings.Contains(h.Statement, needle) {
			return true
		}
	}
	return false
}
