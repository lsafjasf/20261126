package scheduler

import (
	"fmt"
	"strings"
)

// FormatText renders a compact human-readable report.
func FormatText(result *Result) string {
	var out strings.Builder
	summary := result.Summary
	fmt.Fprintf(
		&out,
		"排程完成：%d 条描述，长版 %d，短版 %d，舍弃 %d；合并约束 %d 条。\n",
		summary.Cues, summary.LongPlaced, summary.ShortPlaced, summary.Omitted, summary.Constraints,
	)
	if len(result.Placements) > 0 {
		out.WriteString("\n已排：\n")
		for _, placement := range result.Placements {
			fmt.Fprintf(
				&out,
				"- %s [%s] %s–%s：%s\n",
				placement.CueID,
				placement.Variant,
				formatMS(placement.StartMS),
				formatMS(placement.EndMS),
				placement.Explanation,
			)
		}
	}
	if len(result.Omitted) > 0 {
		out.WriteString("\n舍弃：\n")
		for _, omission := range result.Omitted {
			fmt.Fprintf(&out, "- %s：%s\n", omission.CueID, omission.Explanation)
		}
	}
	return out.String()
}
