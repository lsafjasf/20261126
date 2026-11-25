package framealign

import "fmt"

// EventKind 区分需要落点时间线的事件类型。
type EventKind string

const (
	KindCut         EventKind = "cut"          // 切点
	KindSubtitleIn  EventKind = "subtitle-in"  // 字幕入点
	KindSubtitleOut EventKind = "subtitle-out" // 字幕出点
	KindMarker      EventKind = "marker"       // 标记
)

// Event 描述一个来自源素材、需要落到时间线上的点。
// 位置以源帧号（整数）表达；时间码解析见 timecode.go。
type Event struct {
	Kind        EventKind
	Label       string    // 人工可读标识，如 "A 镜切点"
	SourceFPS   FrameRate // 源素材帧率
	SourceFrame int64     // 源帧号（0 起）
}

// Policy 是候选取整策略的标识。
type Policy string

const (
	PolicyFloor   Policy = "floor"   // 向早取整：不晚于精确点
	PolicyCeil    Policy = "ceil"    // 向晚取整：不早于精确点
	PolicyNearest Policy = "nearest" // 最近帧；恰半帧时向偶数帧取整（银行家舍入，消除系统性偏置）
)

// Candidate 是一个可解释的合法落点候选。
type Candidate struct {
	Policy    Policy
	Frame     int64 // 时间线帧号（合法整数落点）
	ErrFrames Rat   // 相对精确点的偏差（时间线帧，带符号）
	ErrMillis float64
	Explain   string // 该候选的可解释理由
}

// Alignment 是一个事件映射到时间线后的完整结果。
type Alignment struct {
	Event      Event
	ExactFrame Rat  // 精确时间线帧位置（可为分数帧）
	Exact      bool // 是否恰好落在合法落点上
	Candidates []Candidate
}

// Align 把事件映射到时间线。
// 精确命中整数帧时不产生候选（无需取舍）；否则生成 floor/ceil/nearest
// 三个可解释候选，绝不静默取整。
func (tl Timeline) Align(ev Event) Alignment {
	srcTime := ev.SourceFPS.TimeOf(NewRat(ev.SourceFrame, 1))
	exact := tl.Rate.FrameAt(srcTime)
	al := Alignment{Event: ev, ExactFrame: exact, Exact: tl.IsLegal(exact)}
	if al.Exact {
		return al
	}
	lo, hi := exact.Floor(), exact.Ceil()
	step := tl.LegalStep()
	mk := func(p Policy, f int64, why string) Candidate {
		err := NewRat(f, 1).Sub(exact)
		return Candidate{
			Policy:    p,
			Frame:     f,
			ErrFrames: err,
			ErrMillis: err.Float() * step.Float() * 1000,
			Explain:   why,
		}
	}
	al.Candidates = []Candidate{
		mk(PolicyFloor, lo, "向早取整：落点不晚于创作意图点，适合切点/字幕入点不提前截断上一镜头"),
		mk(PolicyCeil, hi, "向晚取整：落点不早于创作意图点，适合字幕出点不提前消失、标记不漏掉目标画面"),
		mk(PolicyNearest, nearestFrame(exact), "最近帧：绝对误差最小；恰半帧时取偶数帧以消除长片累积偏置"),
	}
	return al
}

// nearestFrame 最近整数；恰好在 .5 时取偶数（round-half-to-even）。
func nearestFrame(r Rat) int64 {
	lo := r.Floor()
	frac := r.Frac()
	half := NewRat(1, 2)
	switch frac.Cmp(half) {
	case -1:
		return lo
	case 1:
		return lo + 1
	default: // 恰半帧
		if lo%2 == 0 {
			return lo
		}
		return lo + 1
	}
}

// Choose 按策略从对齐结果中选出落点；精确命中时三个策略等价。
func (al Alignment) Choose(p Policy) (Candidate, error) {
	if al.Exact {
		f := al.ExactFrame.Num()
		return Candidate{
			Policy: p, Frame: f,
			ErrFrames: NewRat(0, 1), ErrMillis: 0,
			Explain: "精确命中合法落点，无需取舍",
		}, nil
	}
	for _, c := range al.Candidates {
		if c.Policy == p {
			return c, nil
		}
	}
	return Candidate{}, fmt.Errorf("framealign: 未知策略 %q", p)
}
