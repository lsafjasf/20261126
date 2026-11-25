package trackstate

import (
	"errors"
	"fmt"
)

var (
	ErrDuplicateTrackID = errors.New("duplicate track id")
	ErrInvalidTrackID   = errors.New("invalid track id")
	ErrUnknownTrack     = errors.New("unknown track")
)

// Track is a timeline track together with its independent state flags.
type Track struct {
	ID       string `json:"id"`
	Name     string `json:"name,omitempty"`
	State    State  `json:"state"`
	Revision uint64 `json:"revision"`
}

// Project is the in-memory editing project. Track order is preserved.
type Project struct {
	ID     string  `json:"id"`
	Tracks []Track `json:"tracks"`
}

// TrackSnapshot is the lossless state of one track plus its compact
// projection. The source State is retained so a lossy projection never
// destroys ambiguous combinations such as mute+solo+lock.
type TrackSnapshot struct {
	TrackID         string `json:"trackId"`
	Name            string `json:"name,omitempty"`
	Revision        uint64 `json:"revision"`
	State           State  `json:"state"`
	ProjectedStatus Status `json:"projectedStatus"`
	Unrepresentable []Flag `json:"unrepresentable,omitempty"`
}

// Lossless reports whether the compact status can represent this snapshot's
// complete state without losing a flag.
func (s TrackSnapshot) Lossless() bool {
	return len(s.Unrepresentable) == 0
}

// Snapshot is a stable, per-track view of a project.
type Snapshot struct {
	SchemaVersion int             `json:"schemaVersion"`
	ProjectID     string          `json:"projectId"`
	Tracks        []TrackSnapshot `json:"tracks"`
}

const snapshotSchemaVersion = 1

// SnapshotTrack creates an independent state snapshot for one track.
func SnapshotTrack(track Track) TrackSnapshot {
	projection := track.State.Project()
	return TrackSnapshot{
		TrackID:         track.ID,
		Name:            track.Name,
		Revision:        track.Revision,
		State:           track.State,
		ProjectedStatus: projection.Status,
		Unrepresentable: append([]Flag(nil), projection.Unrepresentable...),
	}
}

// SnapshotProject creates one snapshot per track without sharing slices or
// state with the project.
func SnapshotProject(project Project) (Snapshot, error) {
	if err := validateProject(project); err != nil {
		return Snapshot{}, err
	}

	snapshot := Snapshot{
		SchemaVersion: snapshotSchemaVersion,
		ProjectID:     project.ID,
		Tracks:        make([]TrackSnapshot, 0, len(project.Tracks)),
	}
	for _, track := range project.Tracks {
		snapshot.Tracks = append(snapshot.Tracks, SnapshotTrack(track))
	}
	return snapshot, nil
}

func validateProject(project Project) error {
	seen := make(map[string]struct{}, len(project.Tracks))
	for _, track := range project.Tracks {
		if track.ID == "" {
			return ErrInvalidTrackID
		}
		if _, exists := seen[track.ID]; exists {
			return fmt.Errorf("%w: %q", ErrDuplicateTrackID, track.ID)
		}
		seen[track.ID] = struct{}{}
	}
	return nil
}
