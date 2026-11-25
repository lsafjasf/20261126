// Package report renders relink results as machine-readable JSON and a
// human review sheet.
package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"mediarelink/internal/match"
	"mediarelink/internal/project"
)

// Report is the full machine-readable result of a check or apply run.
type Report struct {
	Project      string                `json:"project"`
	MediaRoot    string                `json:"media_root"`
	Summary      match.Summary         `json:"summary"`
	Present      []string              `json:"present_media_ids"`
	Assets       []match.AssetPlan     `json:"assets"`
	SkippedFiles []string              `json:"skipped_files,omitempty"`
	Verification *project.Verification `json:"verification,omitempty"`
}

// WriteJSON stores the report as indented JSON.
func WriteJSON(r Report, path string) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// WriteText renders the human review sheet.
func WriteText(r Report, path string) error {
	var b strings.Builder
	fmt.Fprintf(&b, "relink report for project %q\n", r.Project)
	fmt.Fprintf(&b, "media root: %s\n", r.MediaRoot)
	fmt.Fprintf(&b, "assets: %d total, %d present, %d missing -> %d relinked, %d blocked\n\n",
		r.Summary.Total, r.Summary.OK, r.Summary.Missing, r.Summary.Relinked, r.Summary.Blocked)

	assets := append([]match.AssetPlan(nil), r.Assets...)
	sort.Slice(assets, func(i, j int) bool { return assets[i].MediaID < assets[j].MediaID })
	for _, a := range assets {
		switch a.Status {
		case match.StatusRelinked:
			fmt.Fprintf(&b, "[RELINKED] %s (%s)\n", a.MediaID, a.Name)
			fmt.Fprintf(&b, "    %s -> %s (confidence %.3f)\n", a.OldPath, a.NewPath, a.Confidence)
		default:
			fmt.Fprintf(&b, "[BLOCKED ] %s (%s) reason=%s\n", a.MediaID, a.Name, a.Reason)
			fmt.Fprintf(&b, "    recorded path: %s\n", a.OldPath)
		}
		if len(a.Effects) > 0 {
			fmt.Fprintf(&b, "    effects: %s\n", strings.Join(a.Effects, ", "))
		}
		for _, c := range a.Candidates {
			mark := " "
			note := ""
			if c.Path == a.NewPath {
				mark = "*"
			}
			if c.Rejected {
				note = " REJECTED: " + c.RejectReason
			}
			fmt.Fprintf(&b, "    %s candidate %-40s confidence %.3f%s\n", mark, c.Path, c.Confidence, note)
			for _, e := range c.Evidence {
				fmt.Fprintf(&b, "        %-18s score %.3f weight %.2f  %s\n",
					e.Dimension, e.Score, e.Weight, e.Detail)
			}
		}
		b.WriteByte('\n')
	}

	if r.Verification != nil {
		b.WriteString("traceability verification:\n")
		fmt.Fprintf(&b, "    media ids intact:  %v\n", r.Verification.MediaIDsIntact)
		fmt.Fprintf(&b, "    effects resolve:   %v\n", r.Verification.EffectsResolve)
		fmt.Fprintf(&b, "    relinked hash ok:  %v\n", r.Verification.RelinkedHashOK)
		for _, p := range r.Verification.Problems {
			fmt.Fprintf(&b, "    PROBLEM: %s\n", p)
		}
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// WriteAll writes both report forms into dir and returns their paths.
func WriteAll(r Report, dir string) (jsonPath, txtPath string, err error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}
	jsonPath = filepath.Join(dir, "relink-report.json")
	txtPath = filepath.Join(dir, "relink-report.txt")
	if err := WriteJSON(r, jsonPath); err != nil {
		return "", "", err
	}
	if err := WriteText(r, txtPath); err != nil {
		return "", "", err
	}
	return jsonPath, txtPath, nil
}
