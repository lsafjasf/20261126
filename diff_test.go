package timelinesnap

import "testing"

func TestDiffResourceTableShuffleIsOrderOnly(t *testing.T) {
	old := sampleProject()
	newer := sampleProject()
	newer.Resources[0], newer.Resources[1] = newer.Resources[1], newer.Resources[0]

	report, err := DiffProjects(old, newer)
	if err != nil {
		t.Fatal(err)
	}
	if report.HasContentChanges() {
		t.Fatalf("resource shuffle reported as content change: %+v", report.Changes)
	}
	if len(report.ChangesOf(ChangeOrderOnly)) != 1 {
		t.Fatalf("expected one order-only change, got %+v", report.Changes)
	}

	// Snapshots are identical, so snapshot-level diff is empty.
	oldSnap, _, _ := Build(old)
	newSnap, _, _ := Build(newer)
	snapReport, err := DiffSnapshots(oldSnap, newSnap)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapReport.Changes) != 0 {
		t.Fatalf("normalized snapshots must diff clean, got %+v", snapReport.Changes)
	}
}

func TestDiffMarkerShuffleIsOrderOnly(t *testing.T) {
	old := sampleProject()
	newer := sampleProject()
	newer.Markers[0], newer.Markers[1] = newer.Markers[1], newer.Markers[0]

	report, err := DiffProjects(old, newer)
	if err != nil {
		t.Fatal(err)
	}
	if report.HasContentChanges() {
		t.Fatalf("marker shuffle reported as content change: %+v", report.Changes)
	}
}

func TestDiffUnreachableChurnIsIgnored(t *testing.T) {
	old := sampleProject()
	newer := sampleProject()
	newer.Resources = append(newer.Resources,
		Resource{ID: "r-new-unused", Type: ResourceImage, URI: "s3://x.psd", Size: 1})
	// Also mutate an existing unreachable entry.
	for i := range newer.Resources {
		if newer.Resources[i].ID == "r-unused" {
			newer.Resources[i].Size = 42
		}
	}

	report, err := DiffProjects(old, newer)
	if err != nil {
		t.Fatal(err)
	}
	if report.HasContentChanges() {
		t.Fatalf("unreachable resource churn leaked into diff: %+v", report.Changes)
	}
}

func TestDiffRealResourceEdit(t *testing.T) {
	old := sampleProject()
	newer := sampleProject()
	newer.Resources[0].Checksum = "changed"

	report, err := DiffProjects(old, newer)
	if err != nil {
		t.Fatal(err)
	}
	if !report.HasContentChanges() {
		t.Fatal("checksum edit must be a content change")
	}
	found := false
	for _, c := range report.Changes {
		if c.Type == ChangeModified && c.Path == "resources/r1/checksum" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing checksum modification: %+v", report.Changes)
	}
}

func TestDiffClipReorderIsRealChange(t *testing.T) {
	old := sampleProject()
	newer := sampleProject()
	clips := newer.Tracks[0].Clips
	clips[0], clips[1] = clips[1], clips[0]

	report, err := DiffProjects(old, newer)
	if err != nil {
		t.Fatal(err)
	}
	if !report.HasContentChanges() {
		t.Fatal("clip reorder changes playback and must be a content change")
	}
	if len(report.ChangesOf(ChangeReordered)) == 0 {
		t.Fatalf("expected reordered change, got %+v", report.Changes)
	}
}

func TestDiffEffectReorderIsRealChange(t *testing.T) {
	old := sampleProject()
	old.Tracks[0].Clips[0].Effects = []Effect{
		{ID: "e1", Kind: "blur"},
		{ID: "e2", Kind: "grayscale"},
	}
	newer := sampleProject()
	newer.Tracks[0].Clips[0].Effects = []Effect{
		{ID: "e2", Kind: "grayscale"},
		{ID: "e1", Kind: "blur"},
	}

	report, err := DiffProjects(old, newer)
	if err != nil {
		t.Fatal(err)
	}
	if !report.HasContentChanges() {
		t.Fatal("effect-stack reorder must be a content change")
	}
	if len(report.ChangesOf(ChangeReordered)) == 0 {
		t.Fatalf("expected reordered change, got %+v", report.Changes)
	}
}

func TestDiffTrackStateChange(t *testing.T) {
	old := sampleProject()
	newer := sampleProject()
	newer.Tracks[1].State = TrackDisabled

	report, err := DiffProjects(old, newer)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range report.Changes {
		if c.Path == "tracks/a1/state" && c.Type == ChangeModified {
			found = true
		}
	}
	if !found {
		t.Fatalf("track state change not detected: %+v", report.Changes)
	}
}

func TestDiffAddsAndDeletes(t *testing.T) {
	old := sampleProject()
	newer := sampleProject()
	// Add a marker, delete an effect.
	newer.Markers = append(newer.Markers, Marker{ID: "m3", AtNS: 7, Label: "new"})
	newer.Tracks[0].Clips[0].Effects = nil

	report, err := DiffProjects(old, newer)
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]ChangeType{}
	for _, c := range report.Changes {
		paths[c.Path] = c.Type
	}
	if paths["markers/m3"] != ChangeAdded {
		t.Fatalf("missing marker addition: %+v", report.Changes)
	}
	if paths["tracks/v1/clips/c1/effects/e1"] != ChangeDeleted {
		t.Fatalf("missing effect deletion: %+v", report.Changes)
	}
}

func TestDiffEffectParamsChange(t *testing.T) {
	old := sampleProject()
	newer := sampleProject()
	newer.Tracks[0].Clips[0].Effects[0].Params["radius"] = 8.0

	report, err := DiffProjects(old, newer)
	if err != nil {
		t.Fatal(err)
	}
	if !report.HasContentChanges() {
		t.Fatal("effect param edit must be a content change")
	}
}
