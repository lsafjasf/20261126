package medref

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAddPreservesRawAndMergesSameFile(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "media", "Clip A.MOV"))

	store, err := NewStore(root, Linux)
	if err != nil {
		t.Fatal(err)
	}

	// Lower-case spelling does not exist on the case-sensitive FS, but the
	// on-disk probe finds "Clip A.MOV", so both spellings are one entry.
	r1, err := store.Add("media/Clip A.MOV")
	if err != nil || !r1.Created {
		t.Fatalf("first add: %+v err=%v", r1, err)
	}
	r2, err := store.Add("media/clip a.mov")
	if err != nil || !r2.Merged {
		t.Fatalf("case variant should merge on disk: %+v err=%v", r2, err)
	}
	r3, err := store.Add("media/Clip A.MOV")
	if err != nil || !r3.Duplicate {
		t.Fatalf("exact repeat should be duplicate: %+v", r3)
	}
	if len(store.Manifest().Entries) != 1 {
		t.Fatalf("entries=%d, want 1", len(store.Manifest().Entries))
	}
	entry := store.Manifest().Entries[0]
	if len(entry.Aliases) != 2 || entry.Aliases[0] != "media/Clip A.MOV" ||
		entry.Aliases[1] != "media/clip a.mov" {
		t.Fatalf("raw strings not preserved: %#v", entry.Aliases)
	}
	if entry.Missing {
		t.Error("existing file must not be flagged missing")
	}
}

func TestCaseVariantsAreDistinctFilesOnLinux(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.MOV"))
	writeFile(t, filepath.Join(root, "a.mov"))

	store, err := NewStore(root, Linux)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Add("a.MOV"); err != nil {
		t.Fatal(err)
	}
	r, err := store.Add("a.mov")
	if err != nil {
		t.Fatal(err)
	}
	if !r.Created {
		t.Fatal("two real differently-cased files on Linux must not merge")
	}
	if len(store.Manifest().Entries) != 2 {
		t.Fatalf("entries=%d, want 2", len(store.Manifest().Entries))
	}
}

func TestUnicodeVariantResolvesOnDisk(t *testing.T) {
	root := t.TempDir()
	// Store the composed name; reference the decomposed spelling.
	writeFile(t, filepath.Join(root, "media", "caf\u00e9.mov"))

	store, err := NewStore(root, Linux)
	if err != nil {
		t.Fatal(err)
	}
	r, err := store.Add("media/cafe\u0301.mov")
	if err != nil {
		t.Fatal(err)
	}
	if !r.Created {
		t.Fatal("missing NFC/NFD variants are distinct until disk-probed")
	}
	if r.Entry.Missing {
		t.Error("loose on-disk probe should find the composed file")
	}
	if r.Entry.Resolved == "" || !filepath.IsAbs(r.Entry.Resolved) {
		t.Errorf("resolved path not absolute: %q", r.Entry.Resolved)
	}
}

func TestMissingReferenceIsListed(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root, Linux)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Add("media/ghost.mov"); err != nil {
		t.Fatal(err)
	}
	missing := store.Recheck()
	if len(missing) != 1 || missing[0] != "media/ghost.mov" {
		t.Fatalf("missing=%v", missing)
	}
}

func TestManifestRoundTripPreservesUnicode(t *testing.T) {
	root := t.TempDir()
	manifest := filepath.Join(root, "medref.json")
	store, err := NewStore(root, Linux)
	if err != nil {
		t.Fatal(err)
	}
	raw := "media/한글 caff\u00e9 \u00e9.mov"
	if _, err := store.Add(raw); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(manifest); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	// UTF-8 must be written directly instead of \u escapes.
	if !strings.Contains(string(data), "한글") {
		t.Error("manifest escaped Unicode instead of preserving UTF-8")
	}
	reloaded, err := LoadStore(manifest)
	if err != nil {
		t.Fatal(err)
	}
	got := reloaded.Manifest().Entries[0].Aliases[0]
	if got != raw {
		t.Fatalf("round trip mismatch: %q vs %q", got, raw)
	}
}
