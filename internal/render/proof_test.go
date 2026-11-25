package render

import (
	"os"
	"path/filepath"
	"testing"

	"fontaudit/internal/fontscan"
)

func TestDrawLineProducesTofuBoxes(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "fonts", "NimbusSans-Regular.otf")
	ff, err := fontscan.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	r, err := NewRenderer(ff.Faces[0])
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	proof, err := r.DrawLine(2, "00:00:04.500", "中文 α 混合", out)
	if err != nil {
		t.Fatal(err)
	}
	if len(proof.Boxes) != 4 {
		t.Fatalf("boxes = %v, want the four CJK runes (中文混合)", proof.Boxes)
	}
	if _, err := os.Stat(filepath.Join(out, "cue002.png")); err != nil {
		t.Fatalf("png not written: %v", err)
	}
}

func TestDrawLineAllCoveredHasNoBoxes(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "fonts", "NimbusSans-Regular.otf")
	ff, _ := fontscan.Open(path)
	r, _ := NewRenderer(ff.Faces[0])
	proof, err := r.DrawLine(1, "00:00:01.000", "alpha beta gamma", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(proof.Boxes) != 0 {
		t.Fatalf("unexpected boxes: %v", proof.Boxes)
	}
}
