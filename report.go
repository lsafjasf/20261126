package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// TrackMapping is the repaired identity of one baseline track.
type TrackMapping struct {
	BaselinePosition    int         `json:"baselinePosition"`
	BaselineTrackNumber uint64      `json:"baselineTrackNumber"`
	BaselineTrackUID    uint64      `json:"baselineTrackUid"`
	CurrentPosition     int         `json:"currentPosition"`
	CurrentTrackNumber  uint64      `json:"currentTrackNumber"`
	CurrentTrackUID     uint64      `json:"currentTrackUid"`
	Language            string      `json:"language"`
	CodecID             string      `json:"codecId"`
	CodecPrivateHash    string      `json:"codecPrivateSha256"`
	Default             bool        `json:"default"`
	Score               int         `json:"score"`
	Confidence          string      `json:"confidence"`
	Repairable          bool        `json:"repairable"`
	Evidence            []string    `json:"evidence"`
	Warnings            []string    `json:"warnings,omitempty"`
	Alternatives        []Candidate `json:"alternatives"`
}

// Summary aggregates the repair outcome.
type Summary struct {
	Tracks               int `json:"tracks"`
	Mapped               int `json:"mapped"`
	HighConfidence       int `json:"highConfidence"`
	ManualReview         int `json:"manualReview"`
	References           int `json:"references"`
	RewriteReferences    int `json:"rewriteReferences"`
	KeepReferences       int `json:"keepReferences"`
	UnresolvedReferences int `json:"unresolvedReferences"`
	PatchableInPlace     int `json:"patchableInPlace"`
	UnmatchedTracks      int `json:"unmatchedTracks"`
	Problems             int `json:"problems"`
}

// Report is the top-level mapping document.
type Report struct {
	Schema          string         `json:"schema"`
	Input           string         `json:"input"`
	UIDPolicy       string         `json:"uidPolicy"`
	InputSHA256     string         `json:"inputSha256"`
	TrackMappings   []TrackMapping `json:"trackMappings"`
	ReferenceRepair []Reference    `json:"referenceRepairs"`
	Summary         Summary        `json:"summary"`
}

// buildReport assembles the mapping document from matches and references.
func buildReport(inputName, inputHash, uidPolicy string, matches []MatchResult, refs []Reference) Report {
	report := Report{
		Schema:      "mkv-track-identity/v1",
		Input:       inputName,
		UIDPolicy:   uidPolicy,
		InputSHA256: inputHash,
	}
	summary := Summary{Tracks: len(matches)}
	for _, match := range matches {
		baseline := match.Baseline
		mapping := TrackMapping{
			BaselinePosition:    baseline.Position,
			BaselineTrackNumber: baseline.TrackNumber,
			BaselineTrackUID:    baseline.TrackUID,
			Language:            baseline.effectiveLanguage(),
			CodecID:             baseline.CodecID,
			CodecPrivateHash:    baseline.CodecPrivateHash,
			Default:             baseline.Default,
			Alternatives:        match.All,
		}
		if match.Current != nil {
			summary.Mapped++
			mapping.CurrentPosition = match.Current.Position
			mapping.CurrentTrackNumber = match.Current.TrackNumber
			mapping.CurrentTrackUID = match.Current.TrackUID
			mapping.Score = match.Best.Score
			mapping.Confidence = match.Confidence
			mapping.Repairable = true
			mapping.Evidence = match.Best.Evidence
			mapping.Warnings = match.Best.Conflicts
			if match.Confidence == "high" {
				summary.HighConfidence++
			}
		} else {
			mapping.Confidence = "none"
			mapping.Evidence = []string{}
			summary.UnmatchedTracks++
		}
		report.TrackMappings = append(report.TrackMappings, mapping)
	}
	summary.ManualReview = summary.Tracks - summary.HighConfidence
	summary.References = len(refs)
	for _, ref := range refs {
		switch ref.Action {
		case "rewrite":
			summary.RewriteReferences++
		case "keep":
			summary.KeepReferences++
		default:
			summary.UnresolvedReferences++
		}
		if ref.Patchable {
			summary.PatchableInPlace++
		}
	}
	summary.Problems = summary.UnmatchedTracks + summary.UnresolvedReferences
	for _, ref := range refs {
		if ref.Action == "rewrite" && !ref.Patchable {
			summary.Problems++
		}
	}
	report.ReferenceRepair = refs
	report.Summary = summary
	return report
}

// marshalJSON renders the report with stable two-space indentation.
func marshalJSON(report Report) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(report); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// renderMarkdown renders the human-readable mapping table.
func renderMarkdown(report Report) string {
	var b strings.Builder
	s := report.Summary
	b.WriteString("# Matroska 音轨身份重建映射\n\n")
	fmt.Fprintf(&b, "- 输入：`%s`\n", report.Input)
	fmt.Fprintf(&b, "- SHA-256：`%s`\n", report.InputSHA256)
	fmt.Fprintf(&b, "- 轨道：%d；高置信映射：%d；引用修复：%d 条（重写 %d，保持 %d，未决 %d）\n\n",
		s.Tracks, s.HighConfidence, s.References, s.RewriteReferences, s.KeepReferences, s.UnresolvedReferences)
	b.WriteString("## 轨道映射\n\n")
	b.WriteString("| 旧轨号 | 旧 UID | 语言 | CodecPrivate | Default | 新位置 | 新轨号 | 修复后 UID | 置信度 | 证据 |\n")
	b.WriteString("|---:|---:|---|---|---|---:|---:|---:|---|---|\n")
	for _, m := range report.TrackMappings {
		hash := m.CodecPrivateHash
		if len(hash) > 12 {
			hash = hash[:12]
		}
		fmt.Fprintf(&b, "| %d | `0x%016x` | `%s` | `%s` | %t | %d | %d | `0x%016x` | %s | %s |\n",
			m.BaselineTrackNumber, m.BaselineTrackUID, m.Language, hash, m.Default,
			m.CurrentPosition, m.CurrentTrackNumber, m.CurrentTrackUID,
			m.Confidence, strings.Join(m.Evidence, ", "))
	}
	b.WriteString("\n## 容器引用修复\n\n")
	b.WriteString("| 引用位置 | 类型 | 旧值 | 目标 UID/轨号 | 动作 | 原位补丁 | 原因 |\n")
	b.WriteString("|---|---|---:|---:|---|---|---|\n")
	for _, ref := range report.ReferenceRepair {
		var oldValue, target string
		if ref.Scheme == "uid" {
			oldValue = fmt.Sprintf("`0x%016x`", ref.Value)
			target = fmt.Sprintf("`0x%016x`", ref.TargetUID)
		} else {
			oldValue = fmt.Sprintf("`%d`", ref.Value)
			target = fmt.Sprintf("`%d`", ref.TargetNumber)
		}
		fmt.Fprintf(&b, "| `%s` | %s | %s | %s | `%s` | %t | %s |\n",
			ref.Kind, ref.Scheme, oldValue, target, ref.Action, ref.Patchable, ref.Reason)
	}
	return b.String()
}
