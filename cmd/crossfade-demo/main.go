package main

import (
	"errors"
	"fmt"
	"math"
	"math/rand"
	"time"

	"clipfade"
)

const sr = 48000

type waveFn func(t int64) float64

func sine(freq, amp float64) waveFn {
	return func(t int64) float64 {
		return amp * math.Sin(2*math.Pi*freq*float64(t)/sr)
	}
}

func negate(w waveFn) waveFn {
	return func(t int64) float64 { return -w(t) }
}

func noise(seed int64) waveFn {
	r := rand.New(rand.NewSource(seed))
	return func(int64) float64 { return r.Float64()*2 - 1 }
}

func zcWave(t int64) float64 {
	p := [...]float64{0, 0.7071, 1, 0.7071, 0, -0.7071, -1, -0.7071}
	r := t % 8
	if r < 0 {
		r += 8
	}
	return p[r]
}

func makeClip(id clipfade.ClipID, group clipfade.GroupID, start, inOffset, visibleLen int64,
	mediaLen int, kind clipfade.ClipKind, w waveFn) clipfade.Clip {
	c := clipfade.Clip{
		ID: id, Kind: kind, Group: group,
		Start: start, InOffset: inOffset, VisibleLen: visibleLen,
	}
	if kind == clipfade.KindAudio {
		c.Samples = make([]float64, mediaLen)
		for j := range c.Samples {
			c.Samples[j] = w(c.Start - c.InOffset + int64(j))
		}
	}
	return c
}

// addPair adds an adjacent audio pair at cut with generous centered handles.
func addPair(m *clipfade.Model, cut int64, group clipfade.GroupID, leftID, rightID clipfade.ClipID, wl, wr waveFn) {
	const half = 4800
	const margin = 512
	left := makeClip(leftID, group, 0, 0, cut, int(cut+half+margin), clipfade.KindAudio, wl)
	rightIn := int64(half + margin)
	right := makeClip(rightID, group, cut, rightIn, cut, int(rightIn+cut+margin), clipfade.KindAudio, wr)
	must(m.AddClip(left))
	must(m.AddClip(right))
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func sec(samples int64) string {
	return fmt.Sprintf("%.3fs", float64(samples)/sr)
}

func main() {
	demonstrateMove()
	fmt.Println()
	demonstratePhaseTable()
	fmt.Println()
	demonstrateZeroSnap()
}

func demonstrateMove() {
	fmt.Println("== 1. 200ms 交叉淡化 + 移动画面片段 ==")
	m, err := clipfade.NewModel(sr)
	must(err)
	must(m.AddClip(makeClip("V1", "av", 0, 0, 48000, 0, clipfade.KindVideo, nil)))
	addPair(m, 48000, "av", "A1", "A2", sine(1000, 0.6), sine(1000, 0.6))

	fid, ref1, err := m.AddCrossfade(clipfade.CrossfadeConfig{
		Left: "A1", Right: "A2", Duration: 200 * time.Millisecond,
	})
	must(err)
	c1, err := m.Resolve(ref1)
	must(err)

	d := clipfade.ResolveDuration(200*time.Millisecond, sr)
	fmt.Printf("200ms = %d samples; region [%d,%d) cut=%s rev=%d\n",
		d, c1.Plan.RegionStart, c1.Plan.RegionEnd, sec(c1.Plan.Cut), ref1.Revision)
	fmt.Printf("phase=%s rho=%+.3f -> law=%s; peak=%.3f pass=%v; zc=%d; depth=%.1fdB\n",
		c1.Plan.Phase.Relation, c1.Plan.Phase.Correlation, c1.Plan.Law,
		c1.Report.Peak.Peak, c1.Report.Peak.Pass, c1.Report.ZeroCrossings.Count,
		c1.Report.CancellationDepthDB)
	fmt.Printf("automation points: %d total; first out point sample=%d value=%.2f\n",
		len(c1.Points), c1.Points[0].Sample, c1.Points[0].Value)

	const delta = int64(sr)
	fmt.Printf("\n-- move picture clip V1 by +%s (link group 'av' moves with it) --\n", sec(delta))
	newRev, err := m.MoveClipBy("V1", delta)
	must(err)

	_, staleErr := m.Resolve(ref1)
	fmt.Printf("resolve old reference: isErrStale=%v (%v)\n",
		errors.Is(staleErr, clipfade.ErrStaleReference), staleErr)

	oldStart := c1.Plan.RegionStart
	oldEnd := c1.Plan.RegionEnd
	leftover := 0
	for _, p := range m.AllPoints() {
		if p.Revision != newRev {
			leftover++
		}
		if p.Sample >= oldStart && p.Sample < oldEnd {
			leftover++
		}
	}

	ref2, err := m.Reference(fid)
	must(err)
	c2, err := m.Resolve(ref2)
	must(err)
	fmt.Printf("new rev=%d cut=%s region [%d,%d)\n",
		newRev, sec(ref2.CutSample), c2.Plan.RegionStart, c2.Plan.RegionEnd)
	fmt.Printf("old-generation / old-position points retained: %d\n", leftover)
	fmt.Printf("first out point now sample=%d (shift %+d), value=%.2f\n",
		c2.Points[0].Sample, c2.Points[0].Sample-c1.Points[0].Sample, c2.Points[0].Value)
	fmt.Printf("fresh reference resolves: law=%s phase=%s peakPass=%v\n",
		c2.Plan.Law, c2.Plan.Phase.Relation, c2.Report.Peak.Pass)
}

func demonstratePhaseTable() {
	fmt.Println("== 2. 同相 / 反相 / 不相关 信号判定 ==")
	rows := []struct {
		name     string
		wl, wr   waveFn
		polarity clipfade.PolarityMode
	}{
		{"in-phase", sine(440, 0.7), sine(440, 0.7), clipfade.PolarityPreserve},
		{"anti-phase", sine(330, 0.7), negate(sine(330, 0.7)), clipfade.PolarityPreserve},
		{"anti-phase+flip", sine(330, 0.7), negate(sine(330, 0.7)), clipfade.PolarityCorrectAntiPhase},
		{"uncorrelated", noise(1), noise(2), clipfade.PolarityPreserve},
	}
	fmt.Printf("%-18s %6s %-12s %-11s %6s %5s %8s %s\n",
		"signal", "rho", "relation", "law", "peak", "zc", "dipDB", "note")
	for _, r := range rows {
		m, err := clipfade.NewModel(sr)
		must(err)
		addPair(m, 48000, "", "L", "R", r.wl, r.wr)
		fid, _, err := m.AddCrossfade(clipfade.CrossfadeConfig{
			Left: "L", Right: "R", Duration: 200 * time.Millisecond, Polarity: r.polarity,
		})
		must(err)
		rep, err := m.LastReport(fid)
		must(err)
		note := ""
		if len(rep.Plan.Warnings) > 0 {
			note = rep.Plan.Warnings[0]
		}
		fmt.Printf("%-18s %+6.2f %-12s %-11s %6.3f %5d %8.1f %s\n",
			r.name, rep.Plan.Phase.Correlation, rep.Plan.Phase.Relation, rep.Plan.Law,
			rep.Peak.Peak, rep.ZeroCrossings.Count, rep.CancellationDepthDB, note)
	}

	fmt.Println("\npeak check demo: 0.8 amp in-phase forced to equal-power ->")
	m, err := clipfade.NewModel(sr)
	must(err)
	addPair(m, 48000, "", "L", "R", sine(1000, 0.8), sine(1000, 0.8))
	fid, _, err := m.AddCrossfade(clipfade.CrossfadeConfig{
		Left: "L", Right: "R", Duration: 200 * time.Millisecond, Law: clipfade.LawEqualPower,
	})
	must(err)
	rep, err := m.LastReport(fid)
	must(err)
	fmt.Printf("peak=%.3f ceiling=%.1f overCeilingSamples=%d pass=%v\n",
		rep.Peak.Peak, rep.Peak.Ceiling, rep.Peak.OverCeiling, rep.Peak.Pass)
}

func demonstrateZeroSnap() {
	fmt.Println("== 3. 零交叉检查与吸附 ==")
	m, err := clipfade.NewModel(sr)
	must(err)
	addPair(m, 48011, "", "L", "R", zcWave, zcWave)
	fid, _, err := m.AddCrossfade(clipfade.CrossfadeConfig{
		Left: "L", Right: "R", Duration: 200 * time.Millisecond,
	})
	must(err)

	cut, moved, ref, err := m.SnapCutToZeroCrossing(fid, 64)
	must(err)
	curve, err := m.Resolve(ref)
	must(err)
	ez := curve.Report.EditZeroCrossing
	fmt.Printf("cut 48011 -> %d (moved=%v, %s); region [%d,%d); zc in mixed buffer=%d\n",
		cut, moved, sec(cut), curve.Plan.RegionStart, curve.Plan.RegionEnd,
		curve.Report.ZeroCrossings.Count)
	fmt.Printf("edit zero-crossing: onCut=%v leftDist=%d rightDist=%d\n",
		ez.OnCut, ez.LeftDistance, ez.RightDistance)
}
