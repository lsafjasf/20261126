package fontscan

import (
	"path/filepath"
	"testing"
)

func fixtureFont(t *testing.T) *Face {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", "fonts", "NimbusSans-Regular.otf")
	ff, err := Open(path)
	if err != nil {
		t.Fatalf("open fixture font: %v", err)
	}
	if len(ff.Faces) != 1 {
		t.Fatalf("faces = %d, want 1", len(ff.Faces))
	}
	return ff.Faces[0]
}

func TestGlyphCoverageGreek(t *testing.T) {
	face := fixtureFont(t)
	cov := face.Check([]rune("αβγABC"))
	if len(cov.Missing) != 0 {
		t.Fatalf("greek/latin should be covered, missing=%v", cov.Missing)
	}
}

func TestGlyphCoverageCJKMissing(t *testing.T) {
	face := fixtureFont(t)
	cov := face.Check([]rune("中文α"))
	want := map[rune]bool{'中': true, '文': true}
	if len(cov.Missing) != 2 {
		t.Fatalf("missing=%v, want 中 文", cov.Missing)
	}
	for _, r := range cov.Missing {
		if !want[r] {
			t.Fatalf("unexpected missing rune %q", r)
		}
	}
	if !face.Covers('α') || face.Covers('中') {
		t.Fatal("Covers predicate mismatch")
	}
}

func TestScanDirAndCandidates(t *testing.T) {
	files := ScanDir([]string{filepath.Join("..", "..", "testdata", "fonts", "fallback")})
	if len(files) < 2 {
		t.Fatalf("scanned %d font files, want >=2", len(files))
	}
	primary := fixtureFont(t)
	cands := FindCandidates(files, []rune{'中', '文', '①'}, primary)
	if len(cands) == 0 {
		t.Fatal("expected fallback candidates for CJK runes")
	}
	best := cands[0]
	if best.CoveredCount == 0 {
		t.Fatal("top candidate covers nothing")
	}
	for _, c := range cands {
		if sameFace(c.Face, primary) {
			t.Fatal("primary font must be excluded from candidates")
		}
	}
}

func TestNameTable(t *testing.T) {
	face := fixtureFont(t)
	if face.Family == "" || face.FullName == "" {
		t.Fatalf("name table empty: %+v", face)
	}
	if face.NumGlyphs < 100 {
		t.Fatalf("implausible glyph count %d", face.NumGlyphs)
	}
}

func TestOpenTTCCollection(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "fonts", "collection.ttc")
	ff, err := Open(path)
	if err != nil {
		t.Fatalf("open TTC: %v", err)
	}
	if len(ff.Faces) != 2 {
		t.Fatalf("faces = %d, want 2", len(ff.Faces))
	}
	for i, face := range ff.Faces {
		if face.Index != i || face.Collection == "" {
			t.Fatalf("face metadata wrong: %+v", face)
		}
		if !face.Covers('α') {
			t.Fatalf("face %d should cover Greek alpha", i)
		}
	}
	if ff.Faces[0].FullName == ff.Faces[1].FullName {
		t.Fatal("expected distinct regular/bold face names")
	}
}

func TestOpenMissingFile(t *testing.T) {
	if _, err := Open(filepath.Join("..", "..", "testdata", "fonts", "nope.ttf")); err == nil {
		t.Fatal("expected error for missing font")
	}
}
