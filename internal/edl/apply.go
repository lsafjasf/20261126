package edl

import (
	"encoding/json"
	"fmt"
	"os"
)

// AppliedOp is one offline op re-anchored onto its online asset. The edit
// parameters (in/out points, track, note) are carried over untouched; only
// the identity reference moves.
type AppliedOp struct {
	Seq          int    `json:"seq"`
	Action       string `json:"action"`
	OnlineClipID string `json:"online_clip_id"`
	MediaHash    string `json:"media_hash"`
	OnlinePath   string `json:"online_path"`
	InFrame      int64  `json:"in_frame,omitempty"`
	OutFrame     int64  `json:"out_frame,omitempty"`
	NewTrack     string `json:"new_track,omitempty"`
	NewStartMs   int64  `json:"new_start_ms,omitempty"`
	Note         string `json:"note,omitempty"`
	Reason       string `json:"reason"`
}

// AppliedChangeList is the machine-readable output that an online editor can
// import. It only ever contains applied resolutions.
type AppliedChangeList struct {
	SchemaVersion string      `json:"schema_version"`
	ProjectID     string      `json:"project_id"`
	SourceReport  string      `json:"source_report"`
	Ops           []AppliedOp `json:"ops"`
}

// BuildAppliedChangeList projects applied resolutions into importable ops.
// It returns an error if any non-applied resolution is passed in, so callers
// can never silently relink an ambiguous or dead edit.
func BuildAppliedChangeList(rep Report) (AppliedChangeList, error) {
	out := AppliedChangeList{
		SchemaVersion: rep.OnlineVersion,
		ProjectID:     rep.ProjectID,
		SourceReport:  "relocation-report.json",
	}
	for _, r := range rep.Applied {
		if r.ResolvedTo == nil {
			return AppliedChangeList{}, fmt.Errorf("applied op seq=%d has no target asset", r.Seq)
		}
		out.Ops = append(out.Ops, AppliedOp{
			Seq:          r.Offline.Seq,
			Action:       r.Offline.Action,
			OnlineClipID: r.ResolvedTo.ClipID,
			MediaHash:    r.ResolvedTo.MediaHash,
			OnlinePath:   r.ResolvedTo.Path,
			InFrame:      r.Offline.InFrame,
			OutFrame:     r.Offline.OutFrame,
			NewTrack:     r.Offline.NewTrack,
			NewStartMs:   r.Offline.NewStartMs,
			Note:         r.Offline.Note,
			Reason:       r.Reason,
		})
	}
	return out, nil
}

// WriteJSON writes any value as indented JSON.
func WriteJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %q: %w", path, err)
	}
	return nil
}
