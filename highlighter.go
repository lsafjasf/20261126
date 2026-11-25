package asskaraoke

import (
	"sort"
	"time"
)

// SylState 是一个音节在当前播放位置的渲染状态。
type SylState int

const (
	SylPending SylState = iota // 还未开始
	SylActive                  // 正在唱
	SylDone                    // 已唱完
)

// SylRender 是单个音节的渲染指令。Fill 仅对 \kf 有意义，取值 [0,1]。
type SylRender struct {
	Syllable Syllable
	State    SylState
	Fill     float64
}

// LineRender 是当前一条对话行的高亮结果。
type LineRender struct {
	Line      *Dialogue
	Style     *Style
	MediaTime time.Duration // 该行参考的媒体时间
	ActiveSyl int           // 正在唱的音节下标，-1 表示无
	Syls      []SylRender
}

// Highlighter 把输出音频样本位置映射成逐字卡拉 OK 高亮。
// 所有时间查询都经过 Timeline，因此变速、跳转后不会产生累积偏移。
type Highlighter struct {
	doc      *Document
	timeline *Timeline
	starts   []int64 // Events 按 Start 排序后的下标索引
}

func NewHighlighter(doc *Document, tl *Timeline) *Highlighter {
	idx := make([]int64, len(doc.Events))
	for i := range idx {
		idx[i] = int64(i)
	}
	sort.Slice(idx, func(a, b int) bool {
		return doc.Events[idx[a]].Start < doc.Events[idx[b]].Start
	})
	return &Highlighter{doc: doc, timeline: tl, starts: idx}
}

func (h *Highlighter) Timeline() *Timeline { return h.timeline }
func (h *Highlighter) Document() *Document { return h.doc }

// ActiveAtMedia 返回媒体时间 media 处仍在显示（Start <= media < End）的事件。
func (h *Highlighter) ActiveAtMedia(media time.Duration) []*Dialogue {
	begin := sort.Search(len(h.starts), func(i int) bool {
		return h.doc.Events[h.starts[i]].Start > media
	})
	var out []*Dialogue
	for i := 0; i < begin; i++ {
		d := h.doc.Events[h.starts[i]]
		if media >= d.Start && media < d.End {
			out = append(out, d)
		}
	}
	return out
}

// RenderAtSample 是音频回调入口：传入设备即将播放的输出样本序号，
// 返回该样本对应的所有活动行高亮。
func (h *Highlighter) RenderAtSample(outputSample int64) []LineRender {
	media := h.timeline.OutputSampleToMedia(outputSample)
	return h.RenderAtMedia(media)
}

// RenderAtMedia 按媒体时间计算高亮（反向映射后可直接复用）。
func (h *Highlighter) RenderAtMedia(media time.Duration) []LineRender {
	lines := h.ActiveAtMedia(media)
	out := make([]LineRender, 0, len(lines))
	for _, line := range lines {
		out = append(out, h.renderLine(line, media))
	}
	return out
}

func (h *Highlighter) renderLine(line *Dialogue, media time.Duration) LineRender {
	r := LineRender{
		Line:      line,
		Style:     h.doc.Styles[line.Style],
		MediaTime: media,
		ActiveSyl: -1,
		Syls:      make([]SylRender, 0, len(line.Syls)),
	}
	for _, syl := range line.Syls {
		sr := SylRender{Syllable: syl}
		switch {
		case syl.Kind == KindNone:
			sr.State = SylPending
		case media < syl.Start:
			sr.State = SylPending
		case media >= syl.End:
			sr.State = SylDone
			sr.Fill = 1
		default:
			sr.State = SylActive
			r.ActiveSyl = syl.Index
			if syl.Kind == KindKF && syl.Duration > 0 {
				sr.Fill = float64(media-syl.Start) / float64(syl.Duration)
				if sr.Fill < 0 {
					sr.Fill = 0
				}
				if sr.Fill > 1 {
					sr.Fill = 1
				}
			} else {
				// \k / \ko 在音节开始瞬间整体变色，无扫过过程。
				sr.Fill = 1
			}
		}
		r.Syls = append(r.Syls, sr)
	}
	return r
}

// SampleBoundaries 返回某条事件所有音节边界对应的输出样本序号（正向映射），
// 可用于驱动渲染器在精确样本点上重绘，而不是轮询。
func (h *Highlighter) SampleBoundaries(line *Dialogue) []int64 {
	seen := make(map[int64]struct{}, len(line.Syls)*2)
	for _, syl := range line.Syls {
		if syl.Kind == KindNone {
			continue
		}
		for _, media := range []time.Duration{syl.Start, syl.End} {
			s := h.timeline.MediaToOutputSample(media)
			seen[s] = struct{}{}
		}
	}
	out := make([]int64, 0, len(seen))
	for s := range seen {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
