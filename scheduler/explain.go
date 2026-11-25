package scheduler

import (
	"fmt"
	"strings"
)

func placementExplanation(cue preparedCue, selected selection, conflicts []Conflict) string {
	slot := fmt.Sprintf("%s–%s", formatMS(selected.startMS), formatMS(selected.endMS))
	if selected.variant == "long" {
		relation := fmt.Sprintf("距锚点 %s", formatMS(intervalDistance(
			selected.startMS, selected.endMS, cue.cue.AnchorMS,
		)))
		if selected.startMS <= cue.cue.AnchorMS && cue.cue.AnchorMS <= selected.endMS {
			relation = "覆盖锚点"
		}
		return fmt.Sprintf("长版已排入 %s，%s。", slot, relation)
	}
	if cue.long == nil {
		return fmt.Sprintf("输入未提供长版，短版已排入 %s。", slot)
	}

	text := fmt.Sprintf(
		"长版在允许窗口 %s–%s 内放不下；改用短版排入 %s。",
		formatMS(cue.window.StartMS), formatMS(cue.window.EndMS), slot,
	)
	if drops := droppedByShort(cue.cue); len(drops) > 0 {
		text += "舍弃长版片段：" + describeDrops(drops) + "。"
	}
	if len(conflicts) > 0 {
		text += "冲突来源：" + describeConflicts(conflicts) + "。"
	}
	return text
}

func omissionExplanation(cue preparedCue, conflicts []Conflict, clearance int64, constraints []Constraint) string {
	needed := int64(0)
	name := "可用版本"
	if cue.short != nil {
		needed, _ = variantDuration(*cue.short)
		name = "短版"
	} else if cue.long != nil {
		needed, _ = variantDuration(*cue.long)
		name = "长版"
	}
	largest := largestGap(cue.window, constraints, clearance)
	text := fmt.Sprintf(
		"窗口 %s–%s 内没有可容纳%s（需 %s）的落点，最大空隙仅 %s；整条描述已舍弃。",
		formatMS(cue.window.StartMS), formatMS(cue.window.EndMS),
		name, formatMS(needed), formatMS(largest),
	)
	if drops := droppedByOmission(cue.cue); len(drops) > 0 {
		text += "舍弃内容：" + describeDrops(drops) + "。"
	}
	if len(conflicts) > 0 {
		text += "冲突来源：" + describeConflicts(conflicts) + "。"
	}
	return text
}

func largestGap(window Window, constraints []Constraint, clearance int64) int64 {
	var largest int64
	for _, gap := range freeGaps(window, constraints, clearance) {
		if size := gap.EndMS - gap.StartMS; size > largest {
			largest = size
		}
	}
	return largest
}

func droppedByShort(cue Cue) []InfoDrop {
	if cue.Long == nil {
		return nil
	}
	long := cue.Long
	if len(long.Segments) == 0 {
		return []InfoDrop{{
			Text:   "长版内容（输入未提供分段）",
			Reason: "long_version_omitted",
		}}
	}
	retained := make(map[string]struct{})
	if cue.Short != nil {
		for _, segment := range cue.Short.Segments {
			retained[segmentKey(segment)] = struct{}{}
		}
	}
	drops := make([]InfoDrop, 0)
	for _, segment := range long.Segments {
		key := segmentKey(segment)
		if _, ok := retained[key]; ok {
			continue
		}
		drops = append(drops, InfoDrop{
			SegmentID: segment.ID,
			Text:      segment.Text,
			Reason:    "not_in_short_version",
		})
	}
	return drops
}

func droppedByOmission(cue Cue) []InfoDrop {
	segments := make([]Segment, 0)
	if cue.Long != nil {
		segments = append(segments, cue.Long.Segments...)
	}
	if cue.Short != nil {
		segments = append(segments, cue.Short.Segments...)
	}
	seen := make(map[string]struct{}, len(segments))
	drops := make([]InfoDrop, 0, len(segments))
	for _, segment := range segments {
		key := segmentKey(segment)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		drops = append(drops, InfoDrop{
			SegmentID: segment.ID,
			Text:      segment.Text,
			Reason:    "cue_omitted",
		})
	}
	if len(drops) > 0 {
		return drops
	}
	return []InfoDrop{{
		Text:   "整条音频描述（输入未提供分段）",
		Reason: "cue_omitted",
	}}
}

func segmentKey(segment Segment) string {
	if id := strings.TrimSpace(segment.ID); id != "" {
		return "id:" + id
	}
	return "text:" + strings.TrimSpace(segment.Text)
}

func describeDrops(drops []InfoDrop) string {
	const limit = 5
	parts := make([]string, 0, min(len(drops), limit))
	for i, drop := range drops {
		if i == limit {
			break
		}
		label := strings.TrimSpace(drop.Text)
		if label == "" {
			label = drop.SegmentID
		}
		if label == "" {
			label = "未命名片段"
		}
		parts = append(parts, label)
	}
	text := strings.Join(parts, "、")
	if len(drops) > limit {
		text += fmt.Sprintf(" 等 %d 项", len(drops))
	}
	return text
}

func describeConflicts(conflicts []Conflict) string {
	const limit = 5
	parts := make([]string, 0, min(len(conflicts), limit))
	for i, conflict := range conflicts {
		if i == limit {
			break
		}
		label := fmt.Sprintf(
			"%s %s（%s–%s）",
			constraintKindLabel(conflict.Kind),
			conflict.ConstraintID,
			formatMS(conflict.StartMS),
			formatMS(conflict.EndMS),
		)
		if text := strings.TrimSpace(conflict.Label); text != "" && text != conflict.ConstraintID {
			label += " " + truncateRunes(text, 24)
		}
		parts = append(parts, label)
	}
	text := strings.Join(parts, "、")
	if len(conflicts) > limit {
		text += fmt.Sprintf(" 等 %d 条", len(conflicts))
	}
	return text
}

func truncateRunes(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "…"
}

func constraintKindLabel(kind ConstraintKind) string {
	switch kind {
	case ConstraintDialogue:
		return "对白"
	case ConstraintSoundEffect:
		return "音效保留区"
	case ConstraintSubtitle:
		return "字幕"
	case ConstraintAudioDesc:
		return "已排音频描述"
	default:
		return string(kind)
	}
}

func formatMS(ms int64) string {
	sign := ""
	if ms < 0 {
		sign = "-"
		ms = -ms
	}
	whole := ms / 1000
	frac := ms % 1000
	if frac == 0 {
		return fmt.Sprintf("%s%ds", sign, whole)
	}
	fraction := strings.TrimRight(fmt.Sprintf("%03d", frac), "0")
	return fmt.Sprintf("%s%d.%ss", sign, whole, fraction)
}
