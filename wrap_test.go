package vttlayout

import "testing"

func TestWrapASCIIWords(t *testing.T) {
	m := DefaultMeter(100) // average char ~58px
	lines := WrapLines("one two three four five", 300, m)
	if len(lines) < 2 {
		t.Fatalf("expected wrapping, got %v", lines)
	}
	for _, ln := range lines {
		if m.Measure(ln) > 300+1 {
			t.Fatalf("line %q overflows width (%.0f)", ln, m.Measure(ln))
		}
	}
}

func TestWrapCJKPerRune(t *testing.T) {
	m := DefaultMeter(100) // every CJK rune is 100px
	lines := WrapLines("一二三四五六七八九十", 400, m)
	if len(lines) != 3 {
		t.Fatalf("want 3 lines (4/4/2), got %d: %v", len(lines), lines)
	}
	if lines[0] != "一二三四" {
		t.Fatalf("unexpected first line %q", lines[0])
	}
}

func TestWrapExplicitNewlines(t *testing.T) {
	lines := WrapLines("short\n\nthird", 1000, DefaultMeter(100))
	if len(lines) != 3 || lines[1] != "" {
		t.Fatalf("explicit blank line lost: %#v", lines)
	}
}

func TestWrapHardLongWord(t *testing.T) {
	m := DefaultMeter(100)
	lines := WrapLines("abcdefghij", 250, m)
	if len(lines) < 3 {
		t.Fatalf("long word should hard split, got %v", lines)
	}
	for _, ln := range lines {
		if m.Measure(ln) > 250+1 {
			t.Fatalf("hard split line overflow %q", ln)
		}
	}
}

func TestIsWideRune(t *testing.T) {
	if !IsWideRune('字') || !IsWideRune('，') {
		t.Fatal("CJK/fullwidth should be wide")
	}
	if IsWideRune('a') || IsWideRune(' ') {
		t.Fatal("ascii should not be wide")
	}
}
