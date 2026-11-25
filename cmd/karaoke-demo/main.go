package main

import (
	"fmt"
	"strings"

	"asskaraoke"
)

const script = `[Script Info]
PlayResX: 1280
PlayResY: 720

[V4+ Styles]
Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding
Style: Main,Arial,48,&H00FFFFFF,&H000000FF,&H00000000,&H64000000,0,0,1,2,1,2,30,30,40,1

[Events]
Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Text
Dialogue: 0,0:00:00.00,0:00:02.00,Main,,0,0,0,{\kf40}Ka {\kf40}ra {\kf40}o {\kf40}ke
`

func main() {
	const hz = 48000
	doc, err := asskaraoke.ParseASS(script)
	if err != nil {
		panic(err)
	}
	tl := asskaraoke.NewTimeline(1.0, hz)
	hl := asskaraoke.NewHighlighter(doc, tl)

	// 模拟音频设备：每 24000 样本（0.5s 墙钟）回调一次。
	// 在墙钟 0.5s 切到 2x，在墙钟 1.0s 切回 0.5x，观察高亮仍与音频对齐。
	for frame := 0; frame < 6; frame++ {
		sample := int64(frame+1) * 24000
		play := tl.OutputSamplesToPlay(float64(sample))
		switch play {
		case 500_000_000: // 0.5s
			tl.SetSpeed(play, 2.0)
		case 1_000_000_000: // 1.0s
			tl.SetSpeed(play, 0.5)
		}
		media := tl.OutputSampleToMedia(sample)
		fmt.Printf("frame %d sample=%7d play=%4.2fs media=%4.2fs rate=%.1f\n",
			frame, sample, play.Seconds(), media.Seconds(), tl.Rate())
		for _, line := range hl.RenderAtSample(sample) {
			fmt.Println("  " + renderLine(line))
		}
	}
}

func renderLine(line asskaraoke.LineRender) string {
	var b strings.Builder
	for _, sr := range line.Syls {
		switch sr.State {
		case asskaraoke.SylDone:
			b.WriteString("[" + sr.Syllable.Text + "]")
		case asskaraoke.SylActive:
			b.WriteString(fmt.Sprintf("<%s %.0f%%>", strings.TrimSpace(sr.Syllable.Text), sr.Fill*100))
		default:
			b.WriteString(" " + sr.Syllable.Text + " ")
		}
	}
	return b.String()
}
