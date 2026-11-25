package trackstate

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"
)

func TestSnapshotProjectCreatesIndependentTrackSnapshots(t *testing.T) {
	t.Parallel()

	project := Project{
		ID: "many-combinations",
		Tracks: []Track{
			{
				ID:       "video",
				Name:     "Video",
				State:    State{Mute: true, Solo: true, Lock: true, Enabled: true},
				Revision: 3,
			},
			{
				ID:       "dialogue",
				State:    State{Solo: true, Lock: true, Enabled: false},
				Revision: 4,
			},
			{
				ID:       "music",
				State:    State{Mute: true, Solo: true, Enabled: true},
				Revision: 5,
			},
			{
				ID:       "effects",
				State:    DefaultState(),
				Revision: 6,
			},
		},
	}

	first, err := SnapshotProject(project)
	if err != nil {
		t.Fatal(err)
	}
	second, err := SnapshotProject(project)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Tracks) != len(project.Tracks) {
		t.Fatalf("snapshot tracks = %d, want %d", len(first.Tracks), len(project.Tracks))
	}

	wantStatuses := []Status{
		StatusLocked,
		StatusDisabled,
		StatusMuted,
		StatusActive,
	}
	wantLost := [][]Flag{
		{FlagMute, FlagSolo},
		{FlagSolo, FlagLock},
		{FlagSolo},
		nil,
	}
	for index := range project.Tracks {
		snapshot := first.Tracks[index]
		if snapshot.TrackID != project.Tracks[index].ID {
			t.Fatalf("track %d id = %q", index, snapshot.TrackID)
		}
		if !snapshot.State.Equal(project.Tracks[index].State) {
			t.Fatalf("track %q lost source flags: %+v", snapshot.TrackID, snapshot.State)
		}
		if snapshot.ProjectedStatus != wantStatuses[index] {
			t.Fatalf(
				"track %q status = %q, want %q",
				snapshot.TrackID,
				snapshot.ProjectedStatus,
				wantStatuses[index],
			)
		}
		if !slices.Equal(snapshot.Unrepresentable, wantLost[index]) {
			t.Fatalf(
				"track %q lost flags = %v, want %v",
				snapshot.TrackID,
				snapshot.Unrepresentable,
				wantLost[index],
			)
		}
	}

	first.Tracks[0].State.Mute = false
	first.Tracks[0].Unrepresentable[0] = FlagLock
	if !project.Tracks[0].State.Mute {
		t.Fatal("mutating a snapshot changed the project")
	}
	if !second.Tracks[0].State.Mute ||
		second.Tracks[0].Unrepresentable[0] != FlagMute {
		t.Fatal("two snapshots share mutable state")
	}
}

func TestApplyPatchOnlyTouchesExplicitTracksAndFields(t *testing.T) {
	t.Parallel()

	project := Project{
		ID: "partial-import",
		Tracks: []Track{
			{
				ID:       "a",
				State:    State{Mute: true, Solo: true, Enabled: true},
				Revision: 4,
			},
			{
				ID:       "b",
				State:    State{Solo: true, Lock: true, Enabled: true},
				Revision: 8,
			},
			{
				ID:       "c",
				State:    State{Enabled: false},
				Revision: 2,
			},
		},
	}

	var patch ImportPatch
	err := json.Unmarshal([]byte(`{
		"projectId": "partial-import",
		"tracks": [
			{"trackId": "b", "state": {"enabled": false}}
		]
	}`), &patch)
	if err != nil {
		t.Fatal(err)
	}

	got, report, err := ApplyPatch(project, patch)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(report.ChangedTracks, []string{"b"}) {
		t.Fatalf("changed tracks = %v, want [b]", report.ChangedTracks)
	}
	if got.Tracks[0] != project.Tracks[0] {
		t.Fatalf("track a changed from %+v to %+v", project.Tracks[0], got.Tracks[0])
	}
	if got.Tracks[2] != project.Tracks[2] {
		t.Fatalf("track c changed from %+v to %+v", project.Tracks[2], got.Tracks[2])
	}

	wantB := Track{
		ID:       "b",
		State:    State{Solo: true, Lock: true, Enabled: false},
		Revision: 9,
	}
	if got.Tracks[1] != wantB {
		t.Fatalf("track b = %+v, want %+v", got.Tracks[1], wantB)
	}
}

func TestApplyPatchNoopKeepsRevision(t *testing.T) {
	t.Parallel()

	project := Project{
		ID: "noop",
		Tracks: []Track{{
			ID:       "track",
			State:    State{Mute: true, Enabled: true},
			Revision: 12,
		}},
	}
	got, report, err := ApplyPatch(project, ImportPatch{
		Tracks: []TrackPatch{{
			TrackID: "track",
			State: StatePatch{
				Mute:    BoolPtr(true),
				Enabled: BoolPtr(true),
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Tracks[0] != project.Tracks[0] {
		t.Fatalf("no-op patch changed track: %+v", got.Tracks[0])
	}
	if len(report.ChangedTracks) != 0 {
		t.Fatalf("no-op changed tracks = %v", report.ChangedTracks)
	}
}

func TestApplyPatchRejectsStaleRevisionAtomically(t *testing.T) {
	t.Parallel()

	project := Project{
		ID: "atomic",
		Tracks: []Track{
			{ID: "a", State: DefaultState(), Revision: 1},
			{ID: "b", State: DefaultState(), Revision: 2},
		},
	}
	stale := uint64(99)
	_, _, err := ApplyPatch(project, ImportPatch{
		Tracks: []TrackPatch{
			{
				TrackID:          "a",
				State:            StatePatch{Mute: BoolPtr(true)},
				ExpectedRevision: &stale,
			},
			{
				TrackID: "b",
				State:   StatePatch{Solo: BoolPtr(true)},
			},
		},
	})
	if !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("error = %v, want ErrRevisionConflict", err)
	}
	if !project.Tracks[0].State.Equal(DefaultState()) ||
		!project.Tracks[1].State.Equal(DefaultState()) {
		t.Fatal("failed import mutated the source project")
	}
}
