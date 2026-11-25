package main

import (
	"fmt"
	"regexp"
	"strconv"
)

// Rate 描述一个候选帧率。Nominal 为时码计数所用的标称帧率，
// Drop 表示是否按 SMPTE 丢帧规则计数（仅 29.97/59.94 使用）。
type Rate struct {
	Name    string
	Nominal int
	FPS     float64
	Drop    bool
}

func (r Rate) String() string { return r.Name }

var candidates = []Rate{
	{Name: "23.976", Nominal: 24, FPS: 23.976},
	{Name: "24", Nominal: 24, FPS: 24},
	{Name: "25", Nominal: 25, FPS: 25},
	{Name: "29.97 NDF", Nominal: 30, FPS: 29.97},
	{Name: "29.97 DF", Nominal: 30, FPS: 29.97, Drop: true},
	{Name: "30", Nominal: 30, FPS: 30},
	{Name: "50", Nominal: 50, FPS: 50},
	{Name: "59.94 NDF", Nominal: 60, FPS: 59.94},
	{Name: "59.94 DF", Nominal: 60, FPS: 59.94, Drop: true},
	{Name: "60", Nominal: 60, FPS: 60},
}

// Timecode 是一个解析后的时码，保留原始字段以便做丢帧证据分析。
type Timecode struct {
	H, M, S, F int
	DropHint   bool // 文本中使用 ';' 分隔帧号
	Raw        string
}

var tcRe = regexp.MustCompile(`\b(\d{1,3})[:;](\d{2})[:;](\d{2})([:;])(\d{2})\b`)

func parseTimecode(s string) (Timecode, error) {
	m := tcRe.FindStringSubmatch(s)
	if m == nil {
		return Timecode{}, fmt.Errorf("无法解析时码 %q", s)
	}
	n := func(i int) int { v, _ := strconv.Atoi(m[i]); return v }
	tc := Timecode{H: n(1), M: n(2), S: n(3), F: n(5), DropHint: m[4] == ";", Raw: m[0]}
	if tc.M > 59 || tc.S > 59 || tc.F > 59 {
		return Timecode{}, fmt.Errorf("时码字段越界 %q", s)
	}
	return tc, nil
}

// frames 将时码换算为绝对帧号。DF 计数按标称率每 30 帧丢 2 帧
// （30→2，60→4），在每分钟边界丢帧、整十分钟除外。
func (t Timecode) frames(r Rate) int64 {
	totalMin := t.H*60 + t.M
	base := int64((totalMin*60+t.S)*r.Nominal + t.F)
	if r.Drop {
		per := r.Nominal / 30 * 2
		base -= int64(per * (totalMin - totalMin/10))
	}
	return base
}

// atDroppedBoundary 报告该时码是否落在 DF 会丢帧的分钟边界上，
// 以及帧号是否属于被丢弃的 00/01。
func (t Timecode) boundaryInfo() (atBoundary bool, droppedRange bool) {
	if t.S != 0 || t.M%10 == 0 {
		return false, false
	}
	return true, t.F < 2
}
