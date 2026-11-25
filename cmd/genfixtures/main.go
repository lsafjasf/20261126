package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"cuemap"
)

func main() {
	outDir := flag.String("out", "testdata", "directory for generated test materials")
	flag.Parse()

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fatal(err)
	}

	file := cuemap.FixtureFile{
		Schema:    "cuemap.fixtures/v1",
		Generator: "go run ./cmd/genfixtures (stdlib only)",
		Fixtures:  fixtures(),
	}
	jsonPath := filepath.Join(*outDir, "cues.json")
	if err := cuemap.WriteFixtures(jsonPath, file); err != nil {
		fatal(err)
	}

	srtPath := filepath.Join(*outDir, "trilingual.srt")
	if err := os.WriteFile(srtPath, []byte(buildSRT(file.Fixtures)), 0o644); err != nil {
		fatal(err)
	}

	fmt.Printf("wrote %s and %s\n", jsonPath, srtPath)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "genfixtures:", err)
	os.Exit(1)
}

func fixtures() []cuemap.Fixture {
	arabicCombining := string([]rune{
		0x0645, 0x064E, 0x0631, 0x0652, 0x062D, 0x064E,
		0x0628, 0x064B, 0x0627,
	})

	htmlRaw := "<ruby>東<rp>(</rp><rt>とう</rt><rp>)</rp></ruby>亜と中文：" +
		"مرحبا 123"
	htmlCue := cuemap.ParseCue("cue-trilingual-ruby", htmlRaw)
	htmlCue.Index = 1
	htmlCue.StartMS = 1000
	htmlCue.EndMS = 5000
	htmlCue.Language = "zh-ar-ja"

	aozoraRaw := "｜長安《ちょうあん》と龍"
	aozoraCue := cuemap.ParseCue("cue-aozora-vertical", aozoraRaw)
	aozoraCue.Index = 2
	aozoraCue.StartMS = 6000
	aozoraCue.EndMS = 9000

	columnCue := cuemap.Cue{
		ID:      "cue-vertical-column-spacing",
		Index:   3,
		StartMS: 9200,
		EndMS:   11200,
		Text:    "中文竖排\n東京",
		Raw:     "中文竖排\\N東京（とうきょう ruby attached to the left column）",
		Mode:    cuemap.ModeVertical,
		Ruby: []cuemap.RubySpan{
			{StartRune: 5, EndRune: 6, Base: "東", Text: "とう"},
			{StartRune: 6, EndRune: 7, Base: "京", Text: "きょう"},
		},
	}

	invalid := cuemap.Cue{
		ID:      "cue-fallback-overlap",
		Index:   4,
		StartMS: 11500,
		EndMS:   13000,
		Text:    "覆蓋",
		Raw:     "RAW: overlapping ruby must return to this cue",
		Ruby: []cuemap.RubySpan{
			{StartRune: 0, EndRune: 1, Text: "ふく"},
			{StartRune: 0, EndRune: 2, Text: "がい"},
		},
	}

	tightMetrics := cuemap.DefaultMetrics()
	tightMetrics.MaxHeight = 1

	return []cuemap.Fixture{
		{
			Name: "arabic-combining",
			Cue: cuemap.Cue{
				ID: "cue-arabic-combining", Index: 0, StartMS: 0, EndMS: 2500,
				Language: "ar", Text: arabicCombining, Raw: arabicCombining,
			},
			Options: cuemap.Options{Direction: cuemap.DirectionAuto, Mode: cuemap.ModeHorizontal},
			WantOK:  true,
		},
		{
			Name:    "trilingual-html-ruby-vertical",
			Cue:     htmlCue,
			Options: cuemap.Options{Mode: cuemap.ModeVertical},
			WantOK:  true,
		},
		{
			Name:    "aozora-ruby-vertical",
			Cue:     aozoraCue,
			Options: cuemap.Options{Mode: cuemap.ModeVertical},
			WantOK:  true,
		},
		{
			Name:    "vertical-column-spacing",
			Cue:     columnCue,
			Options: cuemap.Options{Mode: cuemap.ModeVertical},
			WantOK:  true,
		},
		{
			Name:    "fallback-overlapping-ruby",
			Cue:     invalid,
			Options: cuemap.Options{Mode: cuemap.ModeVertical},
			WantOK:  false,
		},
		{
			Name: "fallback-bounds",
			Cue: cuemap.Cue{
				ID: "cue-fallback-bounds", Index: 5, StartMS: 13500, EndMS: 15000,
				Text: "竖排超高", Raw: "RAW: tight max height must refer back here",
			},
			Options: cuemap.Options{Mode: cuemap.ModeVertical, Metrics: tightMetrics},
			WantOK:  false,
		},
	}
}

func buildSRT(fixtures []cuemap.Fixture) string {
	var b strings.Builder
	for i, fixture := range fixtures {
		cue := fixture.Cue
		if cue.ID == "" {
			continue
		}
		fmt.Fprintf(&b, "%d\n", i+1)
		fmt.Fprintf(&b, "%s --> %s\n", srtTime(cue.StartMS), srtTime(cue.EndMS))
		raw := cue.Raw
		if raw == "" {
			raw = cue.Text
		}
		b.WriteString(raw)
		b.WriteString("\n\n")
	}
	return b.String()
}

func srtTime(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	hours := ms / 3_600_000
	ms -= hours * 3_600_000
	minutes := ms / 60_000
	ms -= minutes * 60_000
	seconds := ms / 1000
	millis := ms % 1000
	return fmt.Sprintf("%02d:%02d:%02d,%03d", hours, minutes, seconds, millis)
}
