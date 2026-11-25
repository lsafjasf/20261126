package medref

import (
	"path/filepath"
	"testing"
)

func TestParseRelative(t *testing.T) {
	cases := []struct {
		raw   string
		clean string
		canon string
	}{
		{"media/a.png", "./media/a.png", "rel://./media/a.png"},
		{"./media/../media/a.png", "./media/a.png", "rel://./media/a.png"},
		{"a/b/../c.png", "./a/c.png", "rel://./a/c.png"},
		{"../outside/a.png", "../outside/a.png", "rel://../outside/a.png"},
		{".", ".", "rel://."},
	}
	for _, c := range cases {
		p := Parse(c.raw)
		if p.Kind != Relative {
			t.Errorf("%q: kind = %d, want Relative", c.raw, p.Kind)
		}
		if p.Clean != c.clean {
			t.Errorf("%q: clean = %q, want %q", c.raw, p.Clean, c.clean)
		}
		if p.Canonical != c.canon {
			t.Errorf("%q: canonical = %q, want %q", c.raw, p.Canonical, c.canon)
		}
		if p.Raw != c.raw {
			t.Errorf("%q: raw not preserved, got %q", c.raw, p.Raw)
		}
	}
}

func TestParseWindowsDrive(t *testing.T) {
	p := Parse(`d:\Media\clip A\mov.mp4`)
	if p.Kind != WindowsDrive || p.Drive != "D" {
		t.Fatalf("kind=%d drive=%q", p.Kind, p.Drive)
	}
	if p.Clean != "/Media/clip A/mov.mp4" {
		t.Fatalf("clean=%q", p.Clean)
	}
	if p.Canonical != "drive://D/MEDIA/CLIP A/MOV.MP4" {
		t.Fatalf("canonical=%q", p.Canonical)
	}

	// Slash form and lower-case drive letter normalize to the same key.
	q := Parse("D:/Media/clip A/mov.mp4")
	if q.Canonical != p.Canonical {
		t.Fatalf("drive spellings diverged: %q vs %q", q.Canonical, p.Canonical)
	}
}

func TestParseUNC(t *testing.T) {
	p := Parse(`\\NAS\Share\media\a.mov`)
	if p.Kind != UNC || p.Server != "NAS" || p.Share != "Share" {
		t.Fatalf("kind=%d server=%q share=%q", p.Kind, p.Server, p.Share)
	}
	want := "unc://nas/share/MEDIA/A.MOV"
	if p.Canonical != want {
		t.Fatalf("canonical=%q want %q", p.Canonical, want)
	}
	q := Parse("//nas/share/media/a.mov")
	if q.Canonical != want {
		t.Fatalf("UNC spellings diverged: %q vs %q", q.Canonical, want)
	}
}

func TestParseUnixRooted(t *testing.T) {
	p := Parse("/mnt/media/../media/a.png")
	if p.Kind != UnixRooted || p.Clean != "/mnt/media/a.png" {
		t.Fatalf("kind=%d clean=%q", p.Kind, p.Clean)
	}
	if p.Canonical != "posix:///mnt/media/a.png" {
		t.Fatalf("canonical=%q", p.Canonical)
	}
}

func TestLexicalCasePolicy(t *testing.T) {
	upper := Parse("D:/Media/A.MOV")
	lower := Parse("d:/media/a.mov")
	if !LexicallyEqual(upper, lower, Windows) {
		t.Error("drive variants should be equal on Windows")
	}
	if !LexicallyEqual(upper, lower, Linux) {
		t.Error("drive variants are always case-insensitive")
	}

	relA := Parse("media/A.MOV")
	relB := Parse("media/a.mov")
	if LexicallyEqual(relA, relB, Linux) {
		t.Error("relative paths must stay case-sensitive on Linux")
	}
	if !LexicallyEqual(relA, relB, Windows) {
		t.Error("relative paths fold case on Windows")
	}
	if !LexicallyEqual(relA, relB, Darwin) {
		t.Error("relative paths fold case on Darwin")
	}

	rootA := Parse("/mnt/A.MOV")
	rootB := Parse("/mnt/a.mov")
	if LexicallyEqual(rootA, rootB, Linux) {
		t.Error("POSIX paths stay case-sensitive on Linux")
	}
	if !LexicallyEqual(rootA, rootB, Darwin) {
		t.Error("POSIX paths fold case on Darwin")
	}
}

func TestDistinctPathsNotMergedLexically(t *testing.T) {
	// Different segments, different relative depth, different drives.
	pairs := [][2]string{
		{"media/a.png", "media/b.png"},
		{"media/a.png", "./a.png"},
		{"D:/a.png", "E:/a.png"},
		{"/mnt/a.png", "/mnt2/a.png"},
		{`\\nas\s\a.png`, `\\nas2\s\a.png`},
	}
	for _, pair := range pairs {
		a, b := Parse(pair[0]), Parse(pair[1])
		if LexicallyEqual(a, b, Windows) {
			t.Errorf("distinct paths merged: %q ~ %q", pair[0], pair[1])
		}
	}
}

func TestWindowsStoreMergesRelativeCaseVariants(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "media", "Clip.MOV"))

	store, err := NewStore(root, Windows)
	if err != nil {
		t.Fatal(err)
	}
	r1, err := store.Add("media/Clip.MOV")
	if err != nil {
		t.Fatal(err)
	}
	r2, err := store.Add("MEDIA/CLIP.mov")
	if err != nil {
		t.Fatal(err)
	}
	if !r2.Merged || r1.Entry != r2.Entry {
		t.Fatal("relative case variants must merge on Windows")
	}
	if len(store.Manifest().Entries) != 1 {
		t.Fatalf("entries=%d, want 1", len(store.Manifest().Entries))
	}
	if got := r1.Entry.Aliases; len(got) != 2 || got[1] != "MEDIA/CLIP.mov" {
		t.Fatalf("raw spelling not kept: %#v", got)
	}
}

func TestParseEmptyAndRoot(t *testing.T) {
	if p := Parse(""); p.Kind != Relative || p.Clean != "." || p.Raw != "" {
		t.Fatalf("empty raw: %+v", p)
	}
	if p := Parse("/"); p.Kind != UnixRooted || p.Clean != "/" {
		t.Fatalf("root: %+v", p)
	}
	if p := Parse("D:/"); p.Kind != WindowsDrive || p.Clean != "/" || p.Drive != "D" {
		t.Fatalf("drive root: %+v", p)
	}
}
