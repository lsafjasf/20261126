package trackstate

import (
	"errors"
	"fmt"
)

var (
	ErrDuplicatePatchTrack = errors.New("duplicate track in import patch")
	ErrProjectMismatch     = errors.New("import patch project mismatch")
	ErrRevisionConflict    = errors.New("track revision conflict")
)

// BoolPtr returns a pointer suitable for an explicitly present StatePatch
// field. A nil field means "leave this flag unchanged".
func BoolPtr(value bool) *bool {
	return &value
}

// StatePatch is a per-field partial update. Nil is deliberately different
// from false: nil preserves the current value, while a pointer to false turns
// the flag off.
type StatePatch struct {
	Mute    *bool `json:"mute,omitempty"`
	Solo    *bool `json:"solo,omitempty"`
	Lock    *bool `json:"lock,omitempty"`
	Enabled *bool `json:"enabled,omitempty"`
}

// Empty reports whether the patch changes no state field.
func (p StatePatch) Empty() bool {
	return p.Mute == nil && p.Solo == nil && p.Lock == nil && p.Enabled == nil
}

// Apply overlays only the fields explicitly present in the patch.
func (p StatePatch) Apply(base State) State {
	if p.Mute != nil {
		base.Mute = *p.Mute
	}
	if p.Solo != nil {
		base.Solo = *p.Solo
	}
	if p.Lock != nil {
		base.Lock = *p.Lock
	}
	if p.Enabled != nil {
		base.Enabled = *p.Enabled
	}
	return base
}

// TrackPatch contains modifications for one track only.
type TrackPatch struct {
	TrackID          string     `json:"trackId"`
	State            StatePatch `json:"state"`
	ExpectedRevision *uint64    `json:"expectedRevision,omitempty"`
}

// ImportPatch is a partial update. Tracks not listed are not touched.
type ImportPatch struct {
	ProjectID string       `json:"projectId,omitempty"`
	Tracks    []TrackPatch `json:"tracks"`
}

// ImportReport describes only tracks whose state actually changed.
type ImportReport struct {
	ChangedTracks []string `json:"changedTracks,omitempty"`
}

// ApplyPatch applies an import patch without changing omitted tracks or
// omitted flags. Validation happens before any state is copied or mutated, so
// an invalid patch cannot leave a partially imported project.
func ApplyPatch(project Project, patch ImportPatch) (Project, ImportReport, error) {
	if err := validateProject(project); err != nil {
		return Project{}, ImportReport{}, err
	}
	if patch.ProjectID != "" && patch.ProjectID != project.ID {
		return Project{}, ImportReport{}, fmt.Errorf(
			"%w: patch %q, project %q",
			ErrProjectMismatch,
			patch.ProjectID,
			project.ID,
		)
	}

	indices := make(map[string]int, len(project.Tracks))
	for index, track := range project.Tracks {
		indices[track.ID] = index
	}
	seen := make(map[string]struct{}, len(patch.Tracks))
	for _, entry := range patch.Tracks {
		if _, exists := seen[entry.TrackID]; exists {
			return Project{}, ImportReport{}, fmt.Errorf(
				"%w: %q",
				ErrDuplicatePatchTrack,
				entry.TrackID,
			)
		}
		seen[entry.TrackID] = struct{}{}

		index, exists := indices[entry.TrackID]
		if !exists {
			return Project{}, ImportReport{}, fmt.Errorf(
				"%w: %q",
				ErrUnknownTrack,
				entry.TrackID,
			)
		}
		current := project.Tracks[index]
		if entry.ExpectedRevision != nil &&
			*entry.ExpectedRevision != current.Revision {
			return Project{}, ImportReport{}, fmt.Errorf(
				"%w: track %q expected %d, got %d",
				ErrRevisionConflict,
				entry.TrackID,
				*entry.ExpectedRevision,
				current.Revision,
			)
		}
	}

	next := cloneProject(project)
	report := ImportReport{}
	for _, entry := range patch.Tracks {
		index := indices[entry.TrackID]
		track := &next.Tracks[index]
		updated := entry.State.Apply(track.State)
		if updated.Equal(track.State) {
			continue
		}
		track.State = updated
		track.Revision++
		report.ChangedTracks = append(report.ChangedTracks, track.ID)
	}
	return next, report, nil
}

func cloneProject(project Project) Project {
	clone := project
	clone.Tracks = append([]Track(nil), project.Tracks...)
	return clone
}
