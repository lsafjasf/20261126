package audit

import (
	"os"
	"path/filepath"
	"testing"

	"fontaudit/internal/fontscan"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(wd, "..", "..")
}

func loadPrimary(t *testing.T) *fontscan.Face {
	root := repoRoot(t)
	ff, err := fontscan.Open(filepath.Join(root, "testdata", "fonts", "NimbusSans-Regular.otf"))
	if err != nil {
		t.Fatal(err)
	}
	return ff.Faces[0]
}

func runAudit(t *testing.T, subtitleName string, proof bool) *Report {
	t.Helper()
	root := repoRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "testdata", "subtitles", subtitleName))
	if err != nil {
		t.Fatal(err)
	}
	opts := Options{
		SubtitlePath: filepath.Join("testdata", "subtitles", subtitleName),
		SubtitleRaw:  raw,
		Primary:      loadPrimary(t),
		ScanDirs:     []string{filepath.Join(root, "testdata", "fonts", "fallback")},
	}
	if proof {
		opts.ProofDir = t.TempDir()
		opts.MaxProofs = 3
	}
	rep, err := Run(opts)
	if err != nil {
		t.Fatal(err)
	}
	return rep
}

func findMissing(rep *Report, r rune) *MissingRune {
	for i := range rep.Missing {
		if rep.Missing[i].Info.Rune == r {
			return &rep.Missing[i]
		}
	}
	return nil
}

func TestAuditSRTCoverageAndFirstOccurrence(t *testing.T) {
	rep := runAudit(t, "lecture.srt", false)
	if rep.CueCount != 6 {
		t.Fatalf("cues = %d", rep.CueCount)
	}
	if rep.DistinctMissing == 0 || rep.TotalMissingHits == 0 {
		t.Fatal("expected missing glyphs")
	}
	// Chinese ideograph must be missing; Greek must be covered.
	if findMissing(rep, '中') == nil {
		t.Error("中 should be missing in Nimbus Sans")
	}
	if findMissing(rep, 'α') != nil {
		t.Error("α should be covered by Nimbus Sans")
	}
	// First occurrence timing evidence.
	mr := findMissing(rep, '第')
	if mr == nil || mr.First.CueIndex != 1 || mr.First.Start != "00:00:01.200" {
		t.Fatalf("first occurrence wrong: %+v", mr)
	}
	if mr.Occurrences < 1 || len(mr.Cues) == 0 {
		t.Fatalf("occurrence accounting wrong: %+v", mr)
	}
}

func TestAuditSubstitutionsAndFallback(t *testing.T) {
	rep := runAudit(t, "lecture.srt", false)
	colon := findMissing(rep, '：')
	if colon == nil {
		t.Fatal("fullwidth colon should be missing")
	}
	var sawAlias, sawNFKC bool
	for _, s := range colon.Substitutions {
		if s.To != ":" {
			t.Fatalf("substitution target = %q", s.To)
		}
		switch s.Kind {
		case "punctuation-alias":
			sawAlias = true
		case "nfkc":
			sawNFKC = true
		}
	}
	if !sawAlias || !sawNFKC {
		t.Fatalf("expected both alias and NFKC substitutions, got %+v", colon.Substitutions)
	}
	if len(colon.FallbackFonts) == 0 {
		t.Fatal("expected at least one fallback font for fullwidth colon")
	}
	// Ranked candidates must include a face that covers the CJK set.
	if len(rep.Candidates) == 0 || rep.Candidates[0].CoveredCount < 10 {
		t.Fatalf("candidate ranking wrong: %+v", rep.Candidates)
	}
}

func TestAuditEncodingVariants(t *testing.T) {
	for _, tc := range []struct {
		file string
		want string
	}{
		{"lecture.utf16le.srt", "utf-16le-bom"},
		{"lecture.utf16be.srt", "utf-16be-bom"},
		{"lecture.gb18030.srt", "gb18030"},
		{"lecture.mojibake.srt", "utf-8"},
	} {
		rep := runAudit(t, tc.file, false)
		if rep.Encoding.Detected != tc.want {
			t.Errorf("%s: encoding=%s want %s", tc.file, rep.Encoding.Detected, tc.want)
		}
		if tc.file == "lecture.mojibake.srt" && !rep.Encoding.Mojibake {
			t.Errorf("%s: mojibake not flagged", tc.file)
		}
	}
}

func TestAuditProofsRendered(t *testing.T) {
	rep := runAudit(t, "lecture.srt", true)
	if len(rep.Proofs) == 0 {
		t.Fatal("expected proof images")
	}
	for _, p := range rep.Proofs {
		if len(p.Boxes) == 0 {
			t.Fatalf("proof cue %d has no tofu boxes", p.CueIndex)
		}
		if _, err := os.Stat(p.PNGPath); err != nil {
			t.Fatalf("proof png missing: %v", err)
		}
	}
}

func TestAuditASSDeclaredFonts(t *testing.T) {
	rep := runAudit(t, "lecture.ass", false)
	if rep.SubtitleFormat != "ass" {
		t.Fatalf("format = %s", rep.SubtitleFormat)
	}
	var sawInline bool
	for _, f := range rep.DeclaredFonts {
		if f.Name == "Droid Sans Fallback" {
			sawInline = true
		}
	}
	if !sawInline {
		t.Fatalf("inline \\fn font not surfaced: %+v", rep.DeclaredFonts)
	}
}

func TestAuditPassWhenFullyCovered(t *testing.T) {
	rep := runAudit(t, "covered.srt", false)
	if rep.DistinctMissing != 0 || rep.TotalMissingHits != 0 {
		t.Fatalf("expected zero missing glyphs, got %d distinct", rep.DistinctMissing)
	}
	if rep.Encoding.ReplacementRunes != 0 {
		t.Fatal("clean UTF-8 subtitle must have no replacement runes")
	}
}
