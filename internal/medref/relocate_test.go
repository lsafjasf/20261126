package medref

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRelocateRebasesRelativeRefs(t *testing.T) {
	oldRoot := t.TempDir()
	writeFile(t, filepath.Join(oldRoot, "media", "a.mov"))
	writeFile(t, filepath.Join(oldRoot, "media", "b.mov"))

	store, err := NewStore(oldRoot, Linux)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"media/a.mov", "media/b.mov", "media/lost.mov"} {
		if _, err := store.Add(raw); err != nil {
			t.Fatal(err)
		}
	}

	newRoot := filepath.Join(t.TempDir(), "moved-project")
	if err := os.MkdirAll(filepath.Join(newRoot, "media"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Move the media tree, including one file, leaving lost.mov behind.
	copyFile(t, filepath.Join(oldRoot, "media", "a.mov"),
		filepath.Join(newRoot, "media", "a.mov"))
	copyFile(t, filepath.Join(oldRoot, "media", "b.mov"),
		filepath.Join(newRoot, "media", "b.mov"))

	report, err := store.Relocate(newRoot)
	if err != nil {
		t.Fatal(err)
	}
	if store.Root() != newRoot || store.Manifest().Project != newRoot {
		t.Fatal("project root not rebased")
	}
	if report.TotalRefs != 3 || len(report.Updated) != 3 {
		t.Fatalf("report=%+v", report)
	}
	if len(report.Missing) != 1 || report.Missing[0] != "media/lost.mov" {
		t.Fatalf("missing=%v", report.Missing)
	}

	// Original relative strings remain unchanged; only the root moved.
	raws := map[string]bool{}
	for _, e := range store.Manifest().Entries {
		raws[e.Aliases[0]] = true
	}
	if !raws["media/a.mov"] || !raws["media/lost.mov"] {
		t.Fatalf("raw strings altered: %v", raws)
	}

	// The existing entry now resolves under the new root.
	res, ok := store.Resolve(Parse("media/a.mov"))
	if !ok || res != filepath.Join(newRoot, "media", "a.mov") {
		t.Fatalf("resolve after relocate=%q ok=%v", res, ok)
	}
}

func TestRelocateUnmappedVolume(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root, Linux)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Add(`D:\media\a.mov`); err != nil {
		t.Fatal(err)
	}
	report, err := store.Relocate(filepath.Join(t.TempDir(), "new"))
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Unmapped) != 1 || report.Unmapped[0] != `D:\media\a.mov` {
		t.Fatalf("unmapped=%v", report.Unmapped)
	}

	// With a mapping, the same reference becomes updatable and probes.
	target := filepath.Join(t.TempDir(), "d-drive")
	writeFile(t, filepath.Join(target, "media", "a.mov"))
	store.SetRootMapping("D:", target)
	r2, err := store.Relocate(filepath.Join(t.TempDir(), "new2"))
	if err != nil {
		t.Fatal(err)
	}
	if len(r2.Unmapped) != 0 || len(r2.Missing) != 0 {
		t.Fatalf("mapped volume should resolve: %+v", r2)
	}
}

func TestDrivePathThroughMapping(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "d")
	writeFile(t, filepath.Join(target, "Media", "Clip A.MOV"))

	store, err := NewStore(root, Linux)
	if err != nil {
		t.Fatal(err)
	}
	store.SetRootMapping("D:", target)

	// Two spellings of the same drive file merge via the lexical policy.
	r1, err := store.Add(`D:\Media\Clip A.MOV`)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := store.Add("d:/media/clip a.mov")
	if err != nil {
		t.Fatal(err)
	}
	if r1.Entry != r2.Entry || !r2.Merged {
		t.Fatal("drive case variants should merge")
	}
	if r1.Entry.Missing {
		t.Error("mapped drive file must resolve on disk")
	}
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
