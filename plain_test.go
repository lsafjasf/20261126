package sdh

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func loadFixtureCues(t *testing.T) []Cue {
	t.Helper()
	path := filepath.Join("testdata", "sdh_mixed.srt")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	cues, err := ParseSRTBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	return cues
}

func TestPlainFixtureCompleteness(t *testing.T) {
	cues := loadFixtureCues(t)
	if len(cues) != 10 {
		t.Fatalf("fixture cues = %d, want 10", len(cues))
	}

	plain := PlainSubtitles(cues)
	report := VerifyCompleteness(cues, plain)
	if !report.Complete {
		t.Fatalf("completeness failed: %+v", report.Missing)
	}
	if report.VocalUnits != 9 || report.Matched != 9 {
		t.Fatalf("vocal units/matched = %d/%d, want 9/9", report.VocalUnits, report.Matched)
	}
	// Three SDH-only cues (door sound, jazz music, knock) disappear, so 7.
	if len(plain) != 7 {
		t.Fatalf("plain cues = %d, want 7:\n%s", len(plain), RenderSRT(plain))
	}
}

func TestPlainPreservesTimingAndID(t *testing.T) {
	cues := loadFixtureCues(t)
	plain := PlainSubtitles(cues)
	byID := map[string]Cue{}
	for _, c := range plain {
		byID[c.ID] = c
	}
	first := cues[0]
	got, ok := byID[first.ID]
	if !ok {
		t.Fatalf("source cue %s missing from plain ids", first.ID)
	}
	if got.Start != first.Start || got.End != first.End {
		t.Fatalf("timing changed: got %v..%v want %v..%v", got.Start, got.End, first.Start, first.End)
	}
}

func TestPlainContainsExpectedLines(t *testing.T) {
	cues := loadFixtureCues(t)
	out := string(RenderSRT(PlainSubtitles(cues)))

	mustContain := []string{
		"你听到了吗？",
		"I still remember that night.",
		"The rain kept falling down",
		"真的吗？",
		"Shine on, shine on",
		"列车即将进站",
		"- Wait!",
		"- I'm coming.",
		"我们走吧。",
	}
	for _, want := range mustContain {
		if !strings.Contains(out, want) {
			t.Errorf("plain output missing %q\n---\n%s", want, out)
		}
	}

	mustNotContain := []string{
		"雷声", "DOOR CREAKS", "jazz music", "敲门声", "笑声",
		"JOHN (V.O.)", "张三：", "李四：", "MARY:",
	}
	for _, banned := range mustNotContain {
		if strings.Contains(out, banned) {
			t.Errorf("plain output must not contain SDH annotation %q\n---\n%s", banned, out)
		}
	}
}

func TestPlainExcludeLyricsDropsOnlyLyrics(t *testing.T) {
	cues := loadFixtureCues(t)
	opts := DefaultPlainOptions()
	opts.ExcludeLyrics = true
	plain := PlainWithOptions(cues, opts)
	report := VerifyCompletenessWith(cues, plain, VerifyOptions{PlainOptions: opts})
	if !report.Complete {
		t.Fatalf("non-lyric vocal content must stay complete: %+v", report.Missing)
	}
	out := string(RenderSRT(plain))
	if strings.Contains(out, "Shine on") || strings.Contains(out, "rain kept falling") {
		t.Fatalf("lyrics should be excluded:\n%s", out)
	}
	if !strings.Contains(out, "I still remember") {
		t.Fatalf("voice-over dialogue must survive:\n%s", out)
	}
}

func TestVerifyDetectsLostDialogue(t *testing.T) {
	cues := loadFixtureCues(t)
	good := PlainSubtitles(cues)

	// Simulate a broken editor that drops the second plain cue.
	bad := append([]Cue(nil), good[0])
	bad = append(bad, good[2:]...)
	report := VerifyCompleteness(cues, bad)
	if report.Complete {
		t.Fatal("expected incompleteness when a cue is dropped")
	}
	if len(report.Missing) == 0 {
		t.Fatal("expected missing items")
	}

	// Simulate corrupted text.
	corrupt := PlainSubtitles(cues)
	corrupt[0].Text = "完全不同的内容"
	report = VerifyCompleteness(cues, corrupt)
	if report.Complete {
		t.Fatal("expected incompleteness when text is corrupted")
	}
}

func TestNormalizeForCompare(t *testing.T) {
	cases := map[string]string{
		"Hello, World!":   "helloworld",
		"你好，世界。":          "你好世界",
		"♪ La-la ♪":       "lala",
		"I'm coming.":     "imcoming",
		"- Wait!\n- Stop": "waitstop",
	}
	for in, want := range cases {
		if got := NormalizeForCompare(in); got != want {
			t.Errorf("NormalizeForCompare(%q) = %q, want %q", in, got, want)
		}
	}
}
