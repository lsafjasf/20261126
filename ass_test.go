package asskaraoke

import (
	"testing"
	"time"
)

const sampleASS = `[Script Info]
PlayResX: 1280
PlayResY: 720

[V4+ Styles]
Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding
Style: Main,Arial,48,&H00FFFFFF,&H000000FF,&H00000000,&H64000000,-1,0,1,2,1,2,30,30,40,1

[Events]
Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Text
Dialogue: 0,0:00:01.00,0:00:03.00,Main,,0,0,0,{\kf20}Hel{\kf30}lo {\ko10}world
Dialogue: 0,0:00:05.00,0:00:06.00,Main,,0,0,0,plain line no karaoke
`

func TestParseStyleAndColor(t *testing.T) {
	doc, err := ParseASS(sampleASS)
	if err != nil {
		t.Fatal(err)
	}
	if doc.PlayResX != 1280 || doc.PlayResY != 720 {
		t.Fatalf("playres = %dx%d", doc.PlayResX, doc.PlayResY)
	}
	st := doc.Styles["Main"]
	if st == nil {
		t.Fatal("missing Main style")
	}
	if st.FontSize != 48 || !st.Bold {
		t.Fatalf("style fields wrong: %+v", st)
	}
	// 次级色 &H000000FF = AABBGGRR -> R=255 红色（已唱色）。
	if st.Secondary != (Color{R: 255, G: 0, B: 0, A: 0}) {
		t.Fatalf("secondary color = %+v", st.Secondary)
	}
}

func TestParseSyllableTiming(t *testing.T) {
	doc, _ := ParseASS(sampleASS)
	line := doc.Events[0]
	if line.Start != time.Second || line.End != 3*time.Second {
		t.Fatalf("line window = %v..%v", line.Start, line.End)
	}
	if len(line.Syls) != 3 {
		t.Fatalf("want 3 syllables got %d: %+v", len(line.Syls), line.Syls)
	}
	want := []struct {
		text            string
		kind            KaraokeKind
		start, end, dur time.Duration
	}{
		{"Hel", KindKF, time.Second, 1200 * time.Millisecond, 200 * time.Millisecond},
		{"lo ", KindKF, 1200 * time.Millisecond, 1500 * time.Millisecond, 300 * time.Millisecond},
		{"world", KindKO, 1500 * time.Millisecond, 1600 * time.Millisecond, 100 * time.Millisecond},
	}
	for i, w := range want {
		got := line.Syls[i]
		if got.Text != w.text || got.Kind != w.kind ||
			got.Start != w.start || got.End != w.end || got.Duration != w.dur {
			t.Fatalf("syl %d = %+v want %+v", i, got, w)
		}
	}
	if line.Plain != "Hello world" {
		t.Fatalf("plain text = %q", line.Plain)
	}
}

func TestHighlightAcrossSpeedSwitch(t *testing.T) {
	doc, _ := ParseASS(sampleASS)
	const hz = 48000
	tl := NewTimeline(1.0, hz)
	hl := NewHighlighter(doc, tl)

	// 第一个音节 [1.0s, 1.2s] 媒体时间。1x 下在墙钟 1.1s 处于 active，
	// \kf 扫过比例应为 0.5。
	renders := hl.RenderAtMedia(1100 * time.Millisecond)
	if len(renders) != 1 {
		t.Fatalf("want 1 active line got %d", len(renders))
	}
	r := renders[0]
	if r.ActiveSyl != 0 || r.Syls[0].Fill < 0.49 || r.Syls[0].Fill > 0.51 {
		t.Fatalf("active syl wrong: %+v", r.Syls[0])
	}

	// 在墙钟 1s（媒体 1s）切到 2x。之后 0.05s 墙钟消费 0.1s 媒体，
	// 因此墙钟 1.05s 听到的是媒体 1.1s。
	tl.SetSpeed(time.Second, 2.0)
	media := tl.OutputSampleToMedia(int64(1.05 * hz))
	if media != 1100*time.Millisecond {
		t.Fatalf("speed-adjusted media = %v", media)
	}
	r2 := hl.RenderAtSample(int64(1.05 * hz))[0]
	if r2.ActiveSyl != 0 {
		t.Fatalf("after speed switch active syl = %d", r2.ActiveSyl)
	}

	// 关键回归：变速前的已唱状态不能被平移。媒体 0.9s 时该行尚未出现，
	// 无论之后怎么变速，该结论必须保持。
	if got := hl.ActiveAtMedia(900 * time.Millisecond); len(got) != 0 {
		t.Fatalf("pre-line history drifted, got %d lines", len(got))
	}

	// 音节边界样本点：首音节起点媒体 1s -> 墙钟 1s -> 样本 48000。
	bounds := hl.SampleBoundaries(doc.Events[0])
	if bounds[0] != hz {
		t.Fatalf("first boundary sample = %d want %d", bounds[0], hz)
	}
}

func TestKaraokeTagWithKT(t *testing.T) {
	doc, err := ParseASS("[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Text\n" +
		"Dialogue: 0,0:00:00.00,0:00:05.00,S,,0,0,0,{\\kt50\\kf10}a{\\kf10}b\n")
	if err != nil {
		t.Fatal(err)
	}
	syls := doc.Events[0].Syls
	// \kt50 把高亮游标重置到行起点后 50cs。
	if syls[0].Start != 500*time.Millisecond || syls[1].Start != 600*time.Millisecond {
		t.Fatalf("kt handling wrong: %+v", syls)
	}
}
