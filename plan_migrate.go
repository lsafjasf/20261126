package safereflow

// buildMigrate translates the unchanged cue box by the smallest delta that
// puts it inside the common safe rectangle (safe for the entire cue).
func (e *Engine) buildMigrate(rc ResolvedCue, f *Finding) *Plan {
	p := e.newPlan(f, ActionMigrate)

	if f.CommonRect == nil || f.CommonRect.Empty() {
		p.Summary = "安全区在 cue 时段内没有公共矩形，单次迁移无法覆盖全程"
		p.Reasons = append(p.Reasons, "no_common_safe_rect", "consider_time_split")
		return p
	}
	common := *f.CommonRect
	dx, dy, fitX, fitY := MinimalTranslation(rc.Box, common)
	if !fitX || !fitY {
		p.Summary = "cue 框比公共安全区更大，原样迁移无法放入"
		p.Reasons = append(p.Reasons, "box_larger_than_safe_rect", "consider_rewrap_or_shorten")
		b := rc.Box
		p.Box = &b
		return p
	}
	if approxEqual(dx, 0) && approxEqual(dy, 0) {
		p.Summary = "cue 已在公共安全区内，无需迁移"
		p.Reasons = append(p.Reasons, "migrate_no_change")
		b := rc.Box
		p.Box = &b
		return p
	}

	target := rc.Box.Translate(dx, dy)
	if !FitsInside(target, common) {
		p.Summary = "最小平移后仍与安全区相交失败"
		p.Reasons = append(p.Reasons, "migrate_validation_failed")
		return p
	}

	p.Feasible = true
	e.finalizeBox(p, rc, target, true)
	p.Summary = "保持字号与排版不变，最小距离迁移到全程公共安全区"
	e.markAuto(p, true)
	if !p.Gate.MigrationAllowed {
		p.Summary = "检测到越界，但安全区置信度不足/覆盖不全：仅给出迁移坐标，禁止自动移动"
	}
	return p
}

// buildTimeSplit handles a moving camera where no single position is safe for
// the whole cue: cut the cue at safe-area boundaries and give each sub-cue its
// own target box.
func (e *Engine) buildTimeSplit(rc ResolvedCue, f *Finding) *Plan {
	p := e.newPlan(f, ActionTimeSplit)

	slices := e.Track.SlicesOver(rc.Cue.TimeRange)
	if len(slices) == 0 {
		p.Reasons = append(p.Reasons, "no_time_slices")
		p.Summary = "cue 时段没有任何安全区数据"
		return p
	}

	var timed []TimedBox
	anyMove := false
	for _, sl := range slices {
		if !sl.Covered || sl.Rect.Empty() {
			p.Feasible = false
			p.Reasons = appendUnique(p.Reasons, "coverage_gap")
			p.Summary = "cue 时段存在安全区覆盖空洞，无法分段迁移"
			p.Segments = nil
			return p
		}
		dx, dy, fitX, fitY := MinimalTranslation(rc.Box, sl.Rect)
		if !fitX || !fitY {
			p.Reasons = append(p.Reasons, "box_larger_than_segment_rect")
			p.Summary = "某些时段的安全区比 cue 框更小，需先拆行/缩短"
			return p
		}
		target := rc.Box.Translate(dx, dy)
		if !FitsInside(target, sl.Rect) {
			p.Reasons = append(p.Reasons, "segment_move_failed")
			p.Summary = "分段迁移校验失败"
			return p
		}
		if !approxEqual(dx, 0) || !approxEqual(dy, 0) {
			anyMove = true
		}
		if e.PixelAlign {
			target = target.RoundToPixel()
			dx = target.X - rc.Box.X
			dy = target.Y - rc.Box.Y
		}
		timed = append(timed, TimedBox{
			TimeRange: sl.TimeRange,
			Rect:      target,
			DX:        dx,
			DY:        dy,
		})
	}

	if !anyMove {
		p.Reasons = append(p.Reasons, "time_split_no_change")
		p.Summary = "各时段均无需移动"
		return p
	}

	merged := mergeTimedBoxes(timed)
	p.Feasible = true
	p.Segments = merged
	p.Summary = "镜头移动导致安全区漂移：按区间拆分 cue 并逐段迁移"
	// Time-split always moves boxes, so it is behind the hard confidence gate.
	e.markAuto(p, true)
	if !p.Gate.MigrationAllowed {
		p.Summary = "可按区间拆分迁移，但安全区置信度不足/覆盖不全：仅输出分段坐标，禁止自动移动"
	}
	return p
}

// mergeTimedBoxes joins adjacent segments that share the same target delta.
func mergeTimedBoxes(in []TimedBox) []TimedBox {
	if len(in) == 0 {
		return in
	}
	out := []TimedBox{in[0]}
	for i := 1; i < len(in); i++ {
		last := &out[len(out)-1]
		cur := in[i]
		sameDelta := approxEqual(last.DX, cur.DX) && approxEqual(last.DY, cur.DY)
		adjacent := approxEqual(last.End, cur.Start)
		if sameDelta && adjacent {
			last.End = cur.End
			continue
		}
		out = append(out, cur)
	}
	return out
}
