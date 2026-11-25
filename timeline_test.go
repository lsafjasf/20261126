package asskaraoke

import (
	"testing"
	"time"
)

func TestConstantRateRoundTrip(t *testing.T) {
	tl := NewTimeline(1.0, 48000)
	for _, media := range []time.Duration{0, 250 * time.Millisecond, 3*time.Second + 7*time.Millisecond} {
		play := tl.MediaToPlay(media)
		if got := tl.PlayToMedia(play); got != media {
			t.Fatalf("round trip media=%v play=%v got=%v", media, play, got)
		}
		sample := tl.MediaToOutputSample(media)
		if got := tl.OutputSampleToMedia(sample); got != media {
			t.Fatalf("sample round trip media=%v sample=%d got=%v", media, sample, got)
		}
	}
}

func TestSpeedSwitchNoDrift(t *testing.T) {
	const rate = 48000
	tl := NewTimeline(1.0, rate)

	// 以 1x 播放到第 10 秒（墙钟），此刻媒体时间也是 10s。
	switchAt := 10 * time.Second
	tl.SetSpeed(switchAt, 2.0)

	// 变速锚点本身不能发生位移。
	if got := tl.PlayToMedia(switchAt); got != switchAt {
		t.Fatalf("anchor moved: want 10s got %v", got)
	}

	// 再以 2x 播放 10 秒墙钟：消费 20 秒媒体，媒体时间应为 30s。
	media := tl.PlayToMedia(20 * time.Second)
	if media != 30*time.Second {
		t.Fatalf("after 2x segment want 30s, got %v", media)
	}

	// 反向映射：30s 媒体应在墙钟 20s 被听到。
	if play := tl.MediaToPlay(30 * time.Second); play != 20*time.Second {
		t.Fatalf("reverse mapping want 20s, got %v", play)
	}

	// 再降回 0.5x，锚点 20s/30s 保持不动；慢放下 10s 墙钟只走 5s 媒体。
	tl.SetSpeed(20*time.Second, 0.5)
	if got := tl.PlayToMedia(30 * time.Second); got != 35*time.Second {
		t.Fatalf("after 0.5x segment want 35s media, got %v", got)
	}

	// 错误做法（整条线性缩放）会把锚点前的 5s 媒体映射到 2.5s 墙钟；
	// 分段映射必须保持变速点之前完全不变。
	if got := tl.MediaToPlay(5 * time.Second); got != 5*time.Second {
		t.Fatalf("pre-switch history drifted: want 5s got %v", got)
	}
}

func TestSampleMappingAcrossSpeedSwitch(t *testing.T) {
	const hz = 48000
	tl := NewTimeline(1.0, hz)
	tl.SetSpeed(10*time.Second, 2.0)

	// 墙钟 20s = 输出样本 960000；2x 段消费 20s 媒体，对应媒体 30s。
	sample := int64(20 * hz)
	if got := tl.OutputSampleToMedia(sample); got != 30*time.Second {
		t.Fatalf("sample->media want 30s got %v", got)
	}
	// 媒体 30s 反向应回到同一样本。
	if got := tl.MediaToOutputSample(30 * time.Second); got != sample {
		t.Fatalf("media->sample want %d got %d", sample, got)
	}
}

func TestSeekResetsHistory(t *testing.T) {
	tl := NewTimeline(1.0, 48000)
	tl.SetSpeed(5*time.Second, 4.0)
	tl.Seek(30*time.Second, 1.0)
	if len(tl.Breakpoints()) != 1 {
		t.Fatalf("seek should drop history, got %d breakpoints", len(tl.Breakpoints()))
	}
	if got := tl.PlayToMedia(0); got != 30*time.Second {
		t.Fatalf("after seek media at play 0 want 30s got %v", got)
	}
}
