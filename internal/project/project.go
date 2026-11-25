// Package project loads the migrated project, rewrites media paths after
// a confident relink and verifies that media IDs and effect references
// stay traceable end to end.
package project

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"mediarelink/internal/match"
)

// Asset is one media entry as stored by the migrated project. Path is
// the pre-migration location; the metadata fields are the identity
// signals recorded before the move.
type Asset struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Path             string `json:"path"`
	Codec            string `json:"codec"`
	DurationMS       int64  `json:"duration_ms"`
	FPSNum           int    `json:"fps_num"`
	FPSDen           int    `json:"fps_den"`
	FrameCount       int    `json:"frame_count"`
	FrameHash        string `json:"frame_hash"`
	AudioFingerprint string `json:"audio_fingerprint"`
}

// Effect references a media asset by ID. Effects never store paths, so a
// correct relink only rewrites Asset.Path and effects keep resolving.
type Effect struct {
	ID      string         `json:"id"`
	Type    string         `json:"type"`
	MediaID string         `json:"media_id"`
	Params  map[string]any `json:"params,omitempty"`
}

// Project is the migrated edit.
type Project struct {
	Name    string   `json:"project"`
	Media   []Asset  `json:"media"`
	Effects []Effect `json:"effects"`
}

// Load reads a project JSON file.
func Load(path string) (*Project, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var p Project
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := p.validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &p, nil
}

func (p *Project) validate() error {
	seen := map[string]bool{}
	for _, a := range p.Media {
		if a.ID == "" {
			return fmt.Errorf("media asset with empty id")
		}
		if seen[a.ID] {
			return fmt.Errorf("duplicate media id %q", a.ID)
		}
		seen[a.ID] = true
	}
	for _, fx := range p.Effects {
		if !seen[fx.MediaID] {
			return fmt.Errorf("effect %q references unknown media id %q", fx.ID, fx.MediaID)
		}
	}
	return nil
}

// Save writes the project as indented JSON.
func (p *Project) Save(path string) error {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// EffectsByMedia indexes effect IDs per media ID for trace reporting.
func (p *Project) EffectsByMedia() map[string][]string {
	out := map[string][]string{}
	for _, fx := range p.Effects {
		out[fx.MediaID] = append(out[fx.MediaID], fx.ID)
	}
	return out
}

// resolve maps a recorded path onto the new media root. The recorded
// path is tried relative to the root and as-is.
func resolve(mediaRoot, recorded string) (string, bool) {
	for _, cand := range []string{filepath.Join(mediaRoot, recorded), recorded} {
		if info, err := os.Stat(cand); err == nil && info.Mode().IsRegular() {
			return cand, true
		}
	}
	return "", false
}

// Partition splits assets into those still present at their recorded
// path and those missing after the migration.
func (p *Project) Partition(mediaRoot string) (present []string, missing []match.Expected) {
	for _, a := range p.Media {
		if _, ok := resolve(mediaRoot, a.Path); ok {
			present = append(present, a.ID)
			continue
		}
		missing = append(missing, match.Expected{
			MediaID:          a.ID,
			Name:             a.Name,
			OldPath:          a.Path,
			Codec:            a.Codec,
			DurationMS:       a.DurationMS,
			FPSNum:           a.FPSNum,
			FPSDen:           a.FPSDen,
			FrameCount:       a.FrameCount,
			FrameHash:        a.FrameHash,
			AudioFingerprint: a.AudioFingerprint,
		})
	}
	return present, missing
}
