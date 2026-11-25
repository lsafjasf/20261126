package runeinfo

import "testing"

func TestLookupBlocks(t *testing.T) {
	cases := []struct {
		r      rune
		block  string
		script string
	}{
		{'中', "CJK Unified Ideographs", "Han"},
		{'α', "Greek and Coptic", "Greek"},
		{'：', "Halfwidth and Fullwidth Forms", "Common"},
		{'、', "CJK Symbols and Punctuation", "Han"},
	}
	for _, tc := range cases {
		got := Lookup(tc.r)
		if got.Block != tc.block || got.Script != tc.script {
			t.Errorf("U+%04X block=%s script=%s, want %s/%s",
				tc.r, got.Block, got.Script, tc.block, tc.script)
		}
		if got.Codepoint == "" || got.Display == "" {
			t.Errorf("U+%04X missing codepoint/display", tc.r)
		}
	}
}

func TestPunctuationAlias(t *testing.T) {
	if got := Lookup('：').PunctAlias; got != ":" {
		t.Fatalf("fullwidth colon alias = %q, want ':'", got)
	}
	if got := Lookup('中').PunctAlias; got != "" {
		t.Fatalf("CJK ideograph should have no punctuation alias, got %q", got)
	}
}

func TestNFKCFold(t *testing.T) {
	folded, ok := NFKCFold('①')
	if !ok || folded != "1" {
		t.Fatalf("① fold = %q ok=%v, want 1", folded, ok)
	}
	folded, ok = NFKCFold('：')
	if !ok || folded != ":" {
		t.Fatalf("fullwidth colon fold = %q ok=%v", folded, ok)
	}
	if _, ok := NFKCFold('中'); ok {
		t.Fatal("plain ideograph must not fold")
	}
}

func TestCategoryName(t *testing.T) {
	if got := CategoryName('①'); got[:2] != "No" {
		t.Fatalf("① category = %q, want No (other number)", got)
	}
	if got := CategoryName('α'); got[:2] != "Ll" {
		t.Fatalf("α category = %q, want Ll", got)
	}
}
