package asskaraoke

import (
	"fmt"
	"math"
	"sort"
	"time"
)

// Breakpoint 是变速锚点：从 Play 开始（对应媒体时间 Media），播放速率变为 Rate。
// 相邻锚点之间播放时间与媒体时间是线性关系，斜率为该段的 Rate。
type Breakpoint struct {
	Play  time.Duration
	Media time.Duration
	Rate  float64
}

// Timeline 维护“实际播放时间（墙钟）↔ 字幕/音频媒体时间 ↔ 输出音频样本”的映射。
//
// 关键点：切换速率时绝不能把整条时间轴按 ratio 线性缩放，否则变速点之前已经
// 发生的高亮也会被平移，且每次切换都会累积误差。正确做法是在切换瞬间插入锚点，
// 锚点两侧各自保持线性，形成分段线性（piecewise linear）映射。
type Timeline struct {
	sampleRate float64
	breakpoint []Breakpoint // 按 Play 严格递增
}

func NewTimeline(initialRate float64, sampleRate int) *Timeline {
	if initialRate <= 0 {
		panic("asskaraoke: rate must be positive")
	}
	if sampleRate <= 0 {
		panic("asskaraoke: sample rate must be positive")
	}
	return &Timeline{
		sampleRate: float64(sampleRate),
		breakpoint: []Breakpoint{{Rate: initialRate}},
	}
}

func (t *Timeline) Rate() float64 { return t.breakpoint[len(t.breakpoint)-1].Rate }

func (t *Timeline) Breakpoints() []Breakpoint {
	out := make([]Breakpoint, len(t.breakpoint))
	copy(out, t.breakpoint)
	return out
}

// segmentAtPlay 返回播放时间 play 所在的分段锚点。
func (t *Timeline) segmentAtPlay(play time.Duration) Breakpoint {
	idx := sort.Search(len(t.breakpoint), func(i int) bool {
		return t.breakpoint[i].Play > play
	}) - 1
	if idx < 0 {
		idx = 0
	}
	return t.breakpoint[idx]
}

// segmentAtMedia 返回媒体时间 media 所在的分段锚点。
func (t *Timeline) segmentAtMedia(media time.Duration) Breakpoint {
	idx := sort.Search(len(t.breakpoint), func(i int) bool {
		return t.breakpoint[i].Media > media
	}) - 1
	if idx < 0 {
		idx = 0
	}
	return t.breakpoint[idx]
}

// SetSpeed 在实际播放时间 atPlay 处把速率切换为 newRate。
// atPlay 通常由音频设备已消费的样本数换算得到，保证锚点与真实播放位置对齐。
func (t *Timeline) SetSpeed(atPlay time.Duration, newRate float64) {
	if newRate <= 0 {
		panic("asskaraoke: rate must be positive")
	}
	last := t.breakpoint[len(t.breakpoint)-1]
	if atPlay < last.Play {
		atPlay = last.Play
	}
	media := t.PlayToMedia(atPlay)
	if newRate == last.Rate && atPlay == last.Play {
		return
	}
	t.breakpoint = append(t.breakpoint, Breakpoint{Play: atPlay, Media: media, Rate: newRate})
}

// Seek 重置时间轴：跳转到媒体时间 toMedia，之后以 rate 播放。历史分段全部丢弃，
// 因为跳转后旧的播放时间锚点已无意义。
func (t *Timeline) Seek(toMedia time.Duration, rate float64) {
	if rate <= 0 {
		rate = t.Rate()
	}
	t.breakpoint = []Breakpoint{{Media: toMedia, Rate: rate}}
}

// PlayToMedia 正向映射：实际播放时间 -> 媒体（字幕/音频源）时间。
func (t *Timeline) PlayToMedia(play time.Duration) time.Duration {
	bp := t.segmentAtPlay(play)
	return bp.Media + time.Duration(float64(play-bp.Play)*bp.Rate)
}

// MediaToPlay 反向映射：媒体时间 -> 实际播放时间。
func (t *Timeline) MediaToPlay(media time.Duration) time.Duration {
	bp := t.segmentAtMedia(media)
	return bp.Play + time.Duration(float64(media-bp.Media)/bp.Rate)
}

// SampleDuration 返回 n 个输出样本占用的播放时间。
func (t *Timeline) SampleDuration(n int64) time.Duration {
	return time.Duration(float64(n) / t.sampleRate * float64(time.Second))
}

// PlayToOutputSamples 播放时间 -> 输出样本序号（浮点，亚样本精度）。
func (t *Timeline) PlayToOutputSamples(play time.Duration) float64 {
	return play.Seconds() * t.sampleRate
}

// OutputSamplesToPlay 输出样本序号 -> 播放时间。
func (t *Timeline) OutputSamplesToPlay(sample float64) time.Duration {
	return time.Duration(sample / t.sampleRate * float64(time.Second))
}

// MediaToOutputSamples 媒体时间 -> 该媒体位置被听到时对应的输出样本序号（浮点）。
func (t *Timeline) MediaToOutputSamples(media time.Duration) float64 {
	return t.PlayToOutputSamples(t.MediaToPlay(media))
}

// OutputSamplesToMedia 输出样本序号 -> 该样本正在播放的媒体时间（正向：样本到源）。
func (t *Timeline) OutputSamplesToMedia(sample float64) time.Duration {
	return t.PlayToMedia(t.OutputSamplesToPlay(sample))
}

// MediaToOutputSample 四舍五入到整数样本序号。
func (t *Timeline) MediaToOutputSample(media time.Duration) int64 {
	return int64(math.Round(t.MediaToOutputSamples(media)))
}

// OutputSampleToMedia 整数样本序号 -> 媒体时间。
func (t *Timeline) OutputSampleToMedia(sample int64) time.Duration {
	return t.OutputSamplesToMedia(float64(sample))
}

func (t *Timeline) String() string {
	return fmt.Sprintf("Timeline(rate=%g, breakpoints=%d)", t.Rate(), len(t.breakpoint))
}
