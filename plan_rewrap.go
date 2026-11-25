package safereflow

import "strings"

// buildRewrap proposes wrapping the cue at the safe width while keeping its
// anchor point. If the wrapped box only fits after an additional minimal
// translation, the plan carries a Move and is gated as a migration.
func (e *Engine) buildRewrap(rc ResolvedCue, f *Finding) *Plan {
	p := e.newPlan(f, ActionRewrap)

	if f.CommonRect == nil || f.CommonRect.Empty() {
		p.Summary = "缺少覆盖整个 cue 时段的公共安全矩形，无法给出拆行方案"
		p.Reasons = append(p.Reasons, "no_common_safe_rect")
		return p
	}
	common := *f.CommonRect
	st := rc.Style

	contentW := common.W - 2*st.PaddingX
	if contentW < st.FontSize {
		p.Summary = "安全区窄于一个字号，拆行也无法容纳"
		p.Reasons = append(p.Reasons, "safe_rect_too_narrow")
		return p
	}

	wrapW := rc.WrapWidth
	if contentW < wrapW {
		wrapW = contentW
	}

	layout, box := rc.layoutCandidate(rc.Cue.Text, wrapW, e.Measurer)
	p.Lines = layout.Lines
	p.Text = strings.Join(layout.Lines, "\n")

	if len(layout.Lines) > st.MaxLines {
		p.Summary = "拆到超过最大行数仍无法进入安全区"
		p.Reasons = append(p.Reasons, "exceeds_max_lines")
		e.finalizeBox(p, rc, box, false)
		return p
	}

	moved := false
	if !FitsInside(box, common) {
		dx, dy, fitX, fitY := MinimalTranslation(box, common)
		if !fitX || !fitY {
			p.Summary = "拆行后的文本块仍大于公共安全区"
			p.Reasons = append(p.Reasons, "rewrapped_box_too_large")
			e.finalizeBox(p, rc, box, false)
			return p
		}
		box = box.Translate(dx, dy)
		moved = dx != 0 || dy != 0
		if !FitsInside(box, common) {
			p.Summary = "拆行后即使平移也无法进入安全区"
			p.Reasons = append(p.Reasons, "rewrap_plus_move_failed")
			e.finalizeBox(p, rc, box, moved)
			return p
		}
	}

	// Rewrap only contributes when the line layout actually changes. A pure
	// translation with unchanged text belongs to the migrate plan.
	if p.Text == rc.Cue.Text {
		p.Feasible = false
		p.Summary = "当前换行已等于安全宽度，纯拆行无改善"
		p.Reasons = append(p.Reasons, "rewrap_no_change")
		return p
	}

	p.Feasible = true
	e.finalizeBox(p, rc, box, moved)
	if moved {
		p.Summary = "按动态安全区宽度重新拆行，并做最小迁移"
		p.Reasons = append(p.Reasons, "includes_move")
	} else {
		p.Summary = "重新拆行即可让整条 cue 落入动态安全区"
	}
	e.markAuto(p, moved)
	return p
}
