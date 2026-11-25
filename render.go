package channelmap

import (
	"encoding/json"
	"fmt"
	"strings"
)

func RenderJSON(r *Report) ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

func RenderText(r *Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "WAV 声道掩码/样本排列诊断（%s）\n", r.Schema)
	fmt.Fprintf(&b, "状态: %s；置信度: %s\n", r.Status, r.Confidence)
	fmt.Fprintf(&b, "结论: %s\n", r.Summary)
	if r.Input != nil {
		fmt.Fprintf(&b, "输入: %s%d bytes, %d ch, %d Hz, %d frames, %d ms\n",
			inputPrefix(r.Input.Path), r.Input.Bytes, r.Input.ChannelCount, r.Input.SampleRate, r.Input.FrameCount, r.Input.DurationMS)
	}

	fmt.Fprintf(&b, "\n[codec 声明]\n")
	fmt.Fprintf(&b, "- format_tag=0x%04x codec=%s (%s), %d-bit, block_align=%d\n", r.Codec.FormatTag, r.Codec.Codec.ID, r.Codec.Codec.Description, r.Codec.BitsPerSample, r.Codec.BlockAlign)
	for _, issue := range r.Codec.Issues {
		fmt.Fprintf(&b, "- 声明问题: %s\n", issue)
	}

	fmt.Fprintf(&b, "\n[掩码与声明线序]\n")
	fmt.Fprintf(&b, "- mask=%s source=%s popcount=%d\n", r.Mask.Value, r.Mask.Source, r.Mask.PopCount)
	if r.Mask.StandardLayout != "" {
		fmt.Fprintf(&b, "- 常见布局: %s\n", r.Mask.StandardLayout)
	}
	fmt.Fprintf(&b, "- 排序规则: %s\n", r.Mask.OrderRule)
	for _, m := range r.DeclaredMapping {
		fmt.Fprintf(&b, "  wire %d -> %-4s (%s) [%s]\n", m.WireIndex, m.Role, m.Label, m.Source)
	}

	if len(r.Features) > 0 {
		fmt.Fprintf(&b, "\n[多声道特征样本]\n")
		for _, f := range r.Features {
			state := "active"
			if !f.Active {
				state = "silent"
			}
			fmt.Fprintf(&b, "  wire %d: %s rms=%.4f dom=%.1fHz low=%.2f zcr=%.1f clip=%.5f\n",
				f.WireIndex, state, f.RMS, f.DominantFrequencyHz, f.LowBandEnergyRatio, f.ZeroCrossingRate, f.ClippingSampleRatio)
		}
	}

	fmt.Fprintf(&b, "\n[可解释映射]\n")
	for _, m := range r.InferredMapping {
		observed := m.InferredRole
		if observed == "" {
			observed = "?"
		}
		fmt.Fprintf(&b, "  wire %d: declared=%-10s observed=%-10s status=%-11s confidence=%s", m.WireIndex, m.DeclaredRole, observed, m.Status, m.Confidence)
		if m.Score > 0 {
			fmt.Fprintf(&b, " score=%.1f", m.Score)
		}
		b.WriteByte('\n')
		for _, ev := range m.Evidence {
			fmt.Fprintf(&b, "      evidence: %s\n", ev)
		}
	}

	if len(r.Conflicts) > 0 {
		fmt.Fprintf(&b, "\n[冲突]\n")
		for _, c := range r.Conflicts {
			fmt.Fprintf(&b, "- %s [%s/%s] %s\n", c.Code, c.Severity, c.Confidence, c.Message)
			for _, ev := range c.Evidence {
				fmt.Fprintf(&b, "    evidence: %s\n", ev)
			}
		}
	}

	if len(r.Undecidable) > 0 {
		fmt.Fprintf(&b, "\n[不可判定项]\n")
		for _, u := range r.Undecidable {
			fmt.Fprintf(&b, "- %s: %s\n", u.Code, u.Message)
			fmt.Fprintf(&b, "    reason: %s\n", u.Reason)
			if len(u.Candidates) > 0 {
				fmt.Fprintf(&b, "    candidates: %s\n", strings.Join(u.Candidates, " ; "))
			}
		}
	}

	if r.Repair != nil {
		fmt.Fprintf(&b, "\n[修复映射]\n")
		fmt.Fprintf(&b, "- mask-order source wires: %v\n", r.Repair.SourceIndicesForMaskOrder)
		for _, route := range r.Repair.WireToSpeaker {
			fmt.Fprintf(&b, "- %s\n", route)
		}
		fmt.Fprintf(&b, "- %s\n", r.Repair.Description)
	}
	return b.String()
}

func inputPrefix(path string) string {
	if path == "" {
		return ""
	}
	return path + " "
}
