package framealign

import "testing"

func TestRatNormalize(t *testing.T) {
	r := NewRat(30000, 1001)
	if r.Num() != 30000 || r.Den() != 1001 {
		t.Fatalf("30000/1001 应已最简，得 %d/%d", r.Num(), r.Den())
	}
	if got := NewRat(50, 20); got.Num() != 5 || got.Den() != 2 {
		t.Fatalf("50/20 应化为 5/2，得 %d/%d", got.Num(), got.Den())
	}
}

func TestRatFloorCeil(t *testing.T) {
	r := NewRat(5, 2)
	if r.Floor() != 2 || r.Ceil() != 3 {
		t.Fatalf("5/2 floor/ceil = %d/%d，期望 2/3", r.Floor(), r.Ceil())
	}
	neg := NewRat(-5, 2)
	if neg.Floor() != -3 || neg.Ceil() != -2 {
		t.Fatalf("-5/2 floor/ceil = %d/%d，期望 -3/-2", neg.Floor(), neg.Ceil())
	}
	if !NewRat(4, 2).IsInt() {
		t.Fatal("4/2 应为整数")
	}
}

func TestNearestFrameHalfToEven(t *testing.T) {
	cases := []struct {
		r    Rat
		want int64
	}{
		{NewRat(5, 2), 2},   // 2.5 → 2（偶）
		{NewRat(7, 2), 4},   // 3.5 → 4（偶）
		{NewRat(13, 5), 3},  // 2.6 → 3
		{NewRat(12, 5), 2},  // 2.4 → 2
		{NewRat(-5, 2), -2}, // -2.5 → -2（偶）
	}
	for _, c := range cases {
		if got := nearestFrame(c.r); got != c.want {
			t.Errorf("nearestFrame(%d/%d) = %d，期望 %d", c.r.Num(), c.r.Den(), got, c.want)
		}
	}
}

func TestExactPeriod(t *testing.T) {
	tl := NewTimeline(FPS25)
	cases := []struct {
		src  FrameRate
		want int64
	}{
		{FPS25, 1},
		{FPS30, 6},
		{FPS24, 24},
		{FPS2997, 1200},
	}
	for _, c := range cases {
		if got := tl.ExactPeriod(c.src); got != c.want {
			t.Errorf("%s→25 精确周期 = %d，期望 %d", c.src.Name, got, c.want)
		}
	}
}

func TestAlignExact(t *testing.T) {
	tl := NewTimeline(FPS25)
	al := tl.Align(Event{Kind: KindCut, Label: "x", SourceFPS: FPS30, SourceFrame: 720})
	if !al.Exact || len(al.Candidates) != 0 {
		t.Fatalf("30fps 第 720 帧应精确命中且无候选，得 exact=%v candidates=%d", al.Exact, len(al.Candidates))
	}
	if al.ExactFrame.Num() != 600 {
		t.Fatalf("精确帧 = %v，期望 600", al.ExactFrame.Float())
	}
}

func TestAlignHalfFrameCandidates(t *testing.T) {
	tl := NewTimeline(FPS25)
	al := tl.Align(Event{Kind: KindMarker, Label: "m", SourceFPS: FPS30, SourceFrame: 3})
	if al.Exact {
		t.Fatal("30fps 第 3 帧在 25 时间线上应为半帧（2.5）")
	}
	if len(al.Candidates) != 3 {
		t.Fatalf("半帧应产生 3 个候选，得 %d", len(al.Candidates))
	}
	byPolicy := map[Policy]Candidate{}
	for _, c := range al.Candidates {
		byPolicy[c.Policy] = c
	}
	if byPolicy[PolicyFloor].Frame != 2 || byPolicy[PolicyCeil].Frame != 3 {
		t.Fatalf("floor/ceil 应为 2/3，得 %d/%d", byPolicy[PolicyFloor].Frame, byPolicy[PolicyCeil].Frame)
	}
	if byPolicy[PolicyNearest].Frame != 2 {
		t.Fatalf("半帧 nearest 应向偶数取整为 2，得 %d", byPolicy[PolicyNearest].Frame)
	}
	// floor 与 ceil 的绝对误差之和应恰为一帧（40ms）。
	sum := byPolicy[PolicyFloor].ErrMillis*-1 + byPolicy[PolicyCeil].ErrMillis
	if sum < 39.999 || sum > 40.001 {
		t.Fatalf("floor/ceil 误差之和应为 40ms，得 %v", sum)
	}
	if byPolicy[PolicyNearest].Explain == "" {
		t.Fatal("候选必须附可解释说明")
	}
}

func TestChooseExactZeroError(t *testing.T) {
	tl := NewTimeline(FPS25)
	al := tl.Align(Event{Kind: KindCut, Label: "x", SourceFPS: FPS25, SourceFrame: 250})
	for _, p := range []Policy{PolicyFloor, PolicyCeil, PolicyNearest} {
		c, err := al.Choose(p)
		if err != nil || c.Frame != 250 || c.ErrMillis != 0 {
			t.Fatalf("精确命中时 %s 应得帧 250 零误差，得 %+v err=%v", p, c, err)
		}
	}
}
