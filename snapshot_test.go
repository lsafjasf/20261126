package timelinesnap

import "testing"

func sampleProject() *Project {
	return &Project{
		Version: 3,
		Name:    "cut v1",
		Resources: []Resource{
			{ID: "r1", Type: ResourceVideo, URI: "s3://a.mp4", Checksum: "aa", Size: 100},
			{ID: "r2", Type: ResourceAudio, URI: "s3://b.wav", Checksum: "bb"},
			{ID: "r-unused", Type: ResourceImage, URI: "s3://huge.psd", Size: 9_000_000_000},
			{ID: "r-unused-2", Type: ResourceImage, URI: "s3://other.psd"},
		},
		Tracks: []Track{
			{
				ID: "v1", Name: "Video", State: TrackEnabled,
				Clips: []Clip{
					{
						ID: "c1", ResourceID: "r1", StartNS: 0, DurationNS: 5_000_000_000,
						Effects: []Effect{
							{ID: "e1", Kind: "blur", Params: map[string]any{"radius": 4.0}},
						},
					},
					{ID: "c2", ResourceID: "r2", StartNS: 5_000_000_000, DurationNS: 2_000_000_000},
				},
			},
			{ID: "a1", Name: "Audio", State: TrackLocked},
		},
		Markers: []Marker{
			{ID: "m1", AtNS: 1_000_000_000, Label: "intro"},
			{ID: "m2", AtNS: 6_000_000_000, Label: "cut"},
		},
	}
}

func TestBuildKeepsOnlyReachableResources(t *testing.T) {
	snap, stats, err := Build(sampleProject())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.TotalResources != 4 || stats.ReachableResources != 2 || stats.DroppedResources != 2 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if snap.ReachableResources != 2 || snap.DroppedResources != 2 {
		t.Fatalf("unexpected snapshot stats: %+v", snap)
	}
	for _, r := range snap.Resources {
		if r.ID == "r-unused" || r.ID == "r-unused-2" {
			t.Fatalf("unreachable resource %s leaked into snapshot", r.ID)
		}
	}
	if snap.Resources[0].ID != "r1" || snap.Resources[1].ID != "r2" {
		t.Fatalf("resources not sorted by ID: %+v", snap.Resources)
	}
	if snap.Markers[0].ID != "m1" || snap.Markers[1].ID != "m2" {
		t.Fatalf("markers not sorted by ID: %+v", snap.Markers)
	}
}

func TestBuildHashIgnoresResourceAndMarkerOrder(t *testing.T) {
	first, _, err := Build(sampleProject())
	if err != nil {
		t.Fatal(err)
	}
	shuffled := sampleProject()
	shuffled.Resources[0], shuffled.Resources[1] = shuffled.Resources[1], shuffled.Resources[0]
	shuffled.Markers[0], shuffled.Markers[1] = shuffled.Markers[1], shuffled.Markers[0]

	second, _, err := Build(shuffled)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("order-only shuffle changed content hash:\n%s\n%s", first.ID, second.ID)
	}
}

func TestBuildHashChangesOnRealContentEdit(t *testing.T) {
	base, _, err := Build(sampleProject())
	if err != nil {
		t.Fatal(err)
	}

	edited := sampleProject()
	edited.Tracks[0].Clips[0].DurationNS = 9_000_000_000
	editedSnap, _, err := Build(edited)
	if err != nil {
		t.Fatal(err)
	}
	if base.ID == editedSnap.ID {
		t.Fatal("duration edit did not change content hash")
	}

	trackState := sampleProject()
	trackState.Tracks[1].State = TrackDisabled
	stateSnap, _, err := Build(trackState)
	if err != nil {
		t.Fatal(err)
	}
	if base.ID == stateSnap.ID {
		t.Fatal("track state change did not change content hash")
	}
}

func TestBuildDanglingReference(t *testing.T) {
	p := sampleProject()
	p.Tracks[0].Clips[0].ResourceID = "ghost"
	snap, stats, err := Build(p)
	if err == nil {
		t.Fatal("expected ReachError for dangling reference")
	}
	reachErr, ok := err.(*ReachError)
	if !ok || len(reachErr.DanglingRefs) != 1 {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.DanglingRefs[0].ResourceID != "ghost" {
		t.Fatalf("unexpected dangling ref: %+v", stats.DanglingRefs)
	}
	// Clip is retained; only the missing resource is absent.
	if len(snap.Tracks[0].Clips) != 2 || snap.ReachableResources != 1 {
		t.Fatalf("dangling build produced wrong snapshot: %+v", snap)
	}
}

func TestRestoreRoundTrip(t *testing.T) {
	original := sampleProject()
	snap, _, err := Build(original)
	if err != nil {
		t.Fatal(err)
	}
	restored := snap.Restore()
	rebuilt, _, err := Build(restored)
	if err != nil {
		t.Fatal(err)
	}
	if rebuilt.ID != snap.ID {
		t.Fatalf("restore round trip changed content: %s != %s", rebuilt.ID, snap.ID)
	}

	// Restore must be an independent copy.
	restored.Tracks[0].Clips[0].DurationNS = 1
	if snap.Tracks[0].Clips[0].DurationNS == 1 {
		t.Fatal("Restore aliased snapshot memory")
	}
}
