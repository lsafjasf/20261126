package framealign

import "testing"

func TestDropFrameRoundTrip(t *testing.T) {
	cases := []struct {
		tc    string
		frame int64
	}{
		{"00:00:00;00", 0},
		{"00:00:59;29", 1799},
		{"00:01:00;02", 1800}, // 00;00/00;01 标签被丢弃
		{"00:10:00;00", 17982},
		{"01:00:00;00", 107892},
	}
	for _, c := range cases {
		got, err := ParseTimecode(FPS2997, c.tc)
		if err != nil {
			t.Fatalf("ParseTimecode(%q) 出错: %v", c.tc, err)
		}
		if got != c.frame {
			t.Errorf("ParseTimecode(%q) = %d，期望 %d", c.tc, got, c.frame)
		}
		if back := FormatTimecode(FPS2997, c.frame); back != c.tc {
			t.Errorf("FormatTimecode(%d) = %q，期望 %q", c.frame, back, c.tc)
		}
	}
}

func TestNonDropTimecode(t *testing.T) {
	got, err := ParseTimecode(FPS25, "00:00:10:00")
	if err != nil || got != 250 {
		t.Fatalf("25fps 00:00:10:00 应为帧 250，得 %d err=%v", got, err)
	}
	if back := FormatTimecode(FPS25, 250); back != "00:00:10:00" {
		t.Fatalf("回写应为 00:00:10:00，得 %q", back)
	}
}

func TestDropRejectedOnNonNTSC(t *testing.T) {
	if _, err := ParseTimecode(FPS25, "00:00:10;00"); err == nil {
		t.Fatal("25fps 不应接受 drop-frame 时间码")
	}
	if _, err := ParseTimecode(FPS2997, "00:00:10:30"); err == nil {
		t.Fatal("帧字段超出标称 30 应报错")
	}
	if _, err := ParseTimecode(FPS2997, "00:01:00;00"); err == nil {
		t.Fatal("被丢弃的 DF 标签应报错")
	}
}

func TestReportSummarize(t *testing.T) {
	tl := NewTimeline(FPS25)
	events := []Event{
		{Kind: KindCut, Label: "exact", SourceFPS: FPS25, SourceFrame: 100},
		{Kind: KindMarker, Label: "half", SourceFPS: FPS30, SourceFrame: 3},
	}
	rep := BuildReport(tl, events)
	sums := rep.Summarize()
	if len(sums) != 3 {
		t.Fatalf("应有 3 个策略汇总，得 %d", len(sums))
	}
	for _, s := range sums {
		if s.Events != 2 || s.Adjusted != 1 {
			t.Errorf("%s: Events/Adjusted = %d/%d，期望 2/1", s.Policy, s.Events, s.Adjusted)
		}
		if s.MaxAbsMs < 19.999 || s.MaxAbsMs > 20.001 {
			t.Errorf("%s: 半帧事件最大误差应为 20ms，得 %.3f", s.Policy, s.MaxAbsMs)
		}
	}
}
