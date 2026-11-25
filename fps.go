package framealign

import "fmt"

// FrameRate 以有理数表示的帧率（帧/秒）。
// 29.97 严格表示为 30000/1001，不用浮点近似。
type FrameRate struct {
	Name string // 展示名，如 "29.97"
	rate Rat    // 帧/秒
	// DropFrame 为 true 时按 NTSC drop-frame 规则解释时间码（仅 30000/1001）。
	DropFrame bool
}

var (
	FPS24   = FrameRate{Name: "24", rate: NewRat(24, 1)}
	FPS25   = FrameRate{Name: "25", rate: NewRat(25, 1)}
	FPS2997 = FrameRate{Name: "29.97", rate: NewRat(30000, 1001), DropFrame: true}
	FPS30   = FrameRate{Name: "30", rate: NewRat(30, 1)}
)

// KnownRates 返回内置帧率表，供 CLI 按名称查找。
func KnownRates() []FrameRate { return []FrameRate{FPS24, FPS25, FPS2997, FPS30} }

// FindRate 按名称（"24"/"25"/"29.97"/"30"）查找帧率。
func FindRate(name string) (FrameRate, error) {
	for _, r := range KnownRates() {
		if r.Name == name {
			return r, nil
		}
	}
	return FrameRate{}, fmt.Errorf("framealign: 未知帧率 %q（可选 24/25/29.97/30）", name)
}

// Rate 返回帧率有理数。
func (f FrameRate) Rate() Rat { return f.rate }

// FrameDuration 返回单帧时长（秒，有理数）。
func (f FrameRate) FrameDuration() Rat { return NewRat(f.rate.den, f.rate.num) }

// FrameAt 返回时刻 t（秒，有理数）在该帧率下的帧位置（可为分数帧）。
func (f FrameRate) FrameAt(tSeconds Rat) Rat { return tSeconds.Div(f.FrameDuration()) }

// TimeOf 返回帧号 n（可为分数帧）对应的时刻（秒，有理数）。
func (f FrameRate) TimeOf(frame Rat) Rat { return frame.Div(f.rate) }

// Timeline 声明时间线帧率及其派生的合法落点规则。
type Timeline struct {
	Rate FrameRate
}

// NewTimeline 构造时间线；本工具面向 25 fps 时间线，但不写死。
func NewTimeline(rate FrameRate) Timeline { return Timeline{Rate: rate} }

// IsLegal 报告时间线帧位置是否为合法落点（整数帧边界）。
func (tl Timeline) IsLegal(frame Rat) bool { return frame.IsInt() }

// LegalStep 返回时间线上相邻两个合法落点的间隔（秒）。
func (tl Timeline) LegalStep() Rat { return tl.Rate.FrameDuration() }

// ExactPeriod 返回源帧率 src 的帧边界在时间线上重新精确对齐的周期：
// 即每多少源帧出现一次整数时间线帧（最小正周期）。
// 例：30→25 为 6 帧，24→25 为 24 帧，29.97→25 为 1200 帧。
func (tl Timeline) ExactPeriod(src FrameRate) int64 {
	// 源第 n 帧的时间线帧位置 = n * tlFps / srcFps（有理数）。
	// 求最小 n>0 使其为整数：n = srcNum*? —— 直接对每帧步长的分母求解。
	step := tl.Rate.rate.Div(src.rate) // 每个源帧对应的时间线帧数
	return step.den
}
