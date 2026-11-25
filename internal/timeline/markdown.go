package timeline

import (
	"fmt"
	"strings"
)

func RenderMarkdown(report Report) string {
	var builder strings.Builder
	builder.WriteString("# 有理数锚定时间轴报告\n\n")
	builder.WriteString(fmt.Sprintf("- 工程：`%s`\n", report.ProjectName))
	builder.WriteString(fmt.Sprintf("- 全局/序列帧率：`%s fps`\n", report.SequenceRate))
	builder.WriteString(fmt.Sprintf("- 标记数：`%d`；需要吸附选择：`%d`；两次往返通过：`%d`\n",
		report.Counts.Markers, report.Counts.NeedingChoice, report.Counts.RoundTripOK))
	builder.WriteString(fmt.Sprintf("- 锚点分布：媒体 `%d`、片段 `%d`、全局 `%d`\n\n",
		report.Counts.ByAnchor[AnchorMedia], report.Counts.ByAnchor[AnchorClip], report.Counts.ByAnchor[AnchorGlobal]))
	renderDurationSection(&builder, report)
	renderAnchorSemantics(&builder)
	renderCandidateSection(&builder, report)
	return builder.String()
}

func renderDurationSection(builder *strings.Builder, report Report) {
	builder.WriteString("## 总时间精确换算\n\n")
	builder.WriteString(fmt.Sprintf("序列总长度 `%d` 帧 = `%s` 秒（有理数精确值）。\n\n",
		report.Duration.SequenceFrames, report.Duration.Seconds.RatString()))
	builder.WriteString("| 目标帧率 | 精确目标帧 | 整数落点 | 最近候选 |\n")
	builder.WriteString("| --- | ---: | :---: | ---: |\n")
	for _, conversion := range report.Duration.Conversions {
		nearest := findDurationCandidate(conversion, PolicyNearest)
		integerPoint := "否"
		if conversion.Exact {
			integerPoint = "是"
		}
		builder.WriteString(fmt.Sprintf("| %s | `%s` | %s | `%d`（误差 `%s` 帧 / %.3f ms） |\n",
			conversion.Rate, conversion.ExactFrame.RatString(), integerPoint,
			nearest.Frame, nearest.DeltaFrames.RatString(), nearest.DeltaMillisDisplay))
	}
	builder.WriteString("\n")
}

func renderAnchorSemantics(builder *strings.Builder) {
	builder.WriteString("## 锚定语义\n\n")
	builder.WriteString("- `media`：权威坐标是源媒体帧；移动片段会随片段实例移动，滑动源入点或变速后仍回到同一媒体帧。\n")
	builder.WriteString("- `clip`：权威坐标是片段本地帧；片段移动时跟随，滑动源入点/变速不改变其全局时间。\n")
	builder.WriteString("- `global`：权威坐标是全局序列帧；片段移动、滑动或变速都不改变其全局时间。\n")
	builder.WriteString("- 候选只作为派生建议，不静默覆盖权威锚点；因此连续两次往返仍回到原帧。\n\n")
}

func renderCandidateSection(builder *strings.Builder, report Report) {
	builder.WriteString("## 跨帧吸附候选\n\n")
	builder.WriteString("每个非整数目标栅格坐标列出早帧 `floor`、最近帧 `nearest`、晚帧 `ceil`；完整外层相邻候选与反向锚点误差见 JSON。\n\n")
	builder.WriteString("| 标记 | 锚点 | 原帧 | 目标 fps | 精确帧 | floor | nearest | ceil | 角色优先 | 可逆反向策略 |\n")
	separator := "| --- | --- | ---: | --- | ---: | ---: | ---: | ---: | --- | --- |\n"
	builder.WriteString(separator)
	for _, marker := range report.Markers {
		for _, placement := range marker.Placements {
			if placement.Exact {
				continue
			}
			floor := findCandidate(placement, PolicyFloor)
			nearest := findCandidate(placement, PolicyNearest)
			ceil := findCandidate(placement, PolicyCeil)
			roleChoice := findCandidate(placement, marker.RolePolicy)
			nearestChoice := nearest
			builder.WriteString(fmt.Sprintf("| `%s` | `%s` | `%d @%s` | `%s` | `%s` | `%d` | `%d` | `%d` | `%d` | `%s` |\n",
				marker.ID, marker.AnchorKind, marker.AnchorFrame, marker.AnchorRate,
				placement.Rate, placement.ExactFrame.RatString(), floor.Frame, nearest.Frame,
				ceil.Frame, roleChoice.Frame, strings.Join(nearestChoice.SafeReversePolicies, ", ")))
		}
	}
}

func findDurationCandidate(conversion DurationConversion, tag string) DurationCandidate {
	for _, candidate := range conversion.Candidates {
		if hasTag(candidate.Tags, tag) {
			return candidate
		}
	}
	return conversion.Candidates[0]
}

func findCandidate(placement Placement, tag string) Candidate {
	for _, candidate := range placement.Candidates {
		if hasTag(candidate.Tags, tag) {
			return candidate
		}
	}
	return placement.Candidates[0]
}

func hasTag(tags []string, wanted string) bool {
	for _, tag := range tags {
		if tag == wanted {
			return true
		}
	}
	return false
}
