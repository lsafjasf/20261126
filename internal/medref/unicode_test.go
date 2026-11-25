package medref

import "testing"

func TestLooseEqualCase(t *testing.T) {
	if !LooseEqual("Clip A.MOV", "clip a.mov") {
		t.Error("ASCII case variants should be loosely equal")
	}
}

func TestLooseEqualAccents(t *testing.T) {
	// "café": composed é (U+00E9) vs e + combining acute (U+0301).
	composed := "caf\u00e9.mov"
	decomposed := "cafe\u0301.mov"
	if composed == decomposed {
		t.Fatal("test inputs must differ literally")
	}
	if !LooseEqual(composed, decomposed) {
		t.Error("NFC/NFD accent spellings should be loosely equal")
	}
}

func TestLooseEqualHangul(t *testing.T) {
	// Precomposed syllable 가 (U+AC00) vs Jamo ᄀ (U+1100) + ᅡ (U+1161).
	composed := "\uac00.mov"
	jamo := "\u1100\u1161.mov"
	if composed == jamo {
		t.Fatal("test inputs must differ literally")
	}
	if !LooseEqual(composed, jamo) {
		t.Error("composed/decomposed Hangul should be loosely equal")
	}
}

func TestLooseEqualKeepsDistinctNames(t *testing.T) {
	distinct := [][2]string{
		{"café.mov", "caf\u00e8.mov"}, // é vs è: different base letters
		{"clip.mov", "clip2.mov"},
		{"가.mov", "나.mov"}, // different syllables
	}
	for _, pair := range distinct {
		if LooseEqual(pair[0], pair[1]) {
			t.Errorf("legal distinct names merged: %q ~ %q", pair[0], pair[1])
		}
	}
}

func TestCanonicalNeverFoldsUnicode(t *testing.T) {
	// Even on a case-insensitive platform, non-ASCII spellings keep their
	// own canonical keys so legal distinct names are not merged lexically.
	a := Parse("D:/media/caf\u00e9.mov")
	b := Parse("D:/media/cafe\u0301.mov")
	if a.Canonical == b.Canonical {
		t.Error("Unicode variants must keep distinct canonical keys")
	}
	// ASCII case is still folded for the same volume.
	c := Parse("D:/media/CAF\u00e9.MOV")
	if a.Canonical != c.Canonical {
		t.Errorf("ASCII case should fold: %q vs %q", a.Canonical, c.Canonical)
	}
}
