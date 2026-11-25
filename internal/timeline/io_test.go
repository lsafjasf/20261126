package timeline

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadInputAppliesAnchorOffsets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "timeline.json")
	input := `{
	  "project_name": "custom",
	  "sequence_rate": "25",
	  "duration_frames": 250,
	  "rates": ["24", "25", "30"],
	  "markers": [{
	    "id": "m1",
	    "label": "媒体标记",
	    "role": "subtitle_in",
	    "anchor_kind": "media",
	    "anchor_frame": 1,
	    "anchor_rate": "25",
	    "global_time_seconds": "1201/25",
	    "role_policy": "floor",
	    "media_offset_frames": 1200
	  }]
	}`
	if err := os.WriteFile(path, []byte(input), 0o644); err != nil {
		t.Fatal(err)
	}

	model, err := ReadInput(path)
	if err != nil {
		t.Fatal(err)
	}
	report, err := BuildReport(model)
	if err != nil {
		t.Fatal(err)
	}
	placement := report.Markers[0].Placements[0]
	if got := placement.Candidates[0].AnchorCoordinate.RatString(); got != "25/24" {
		t.Fatalf("anchor coordinate = %s, want 25/24", got)
	}
	if !report.Markers[0].RoundTripTwice.OK {
		t.Fatalf("round trip failed: %+v", report.Markers[0].RoundTripTwice)
	}
}

func TestRejectInconsistentAnchorFrame(t *testing.T) {
	model := DemoTimeline()
	model.Markers[0].AnchorFrame = 999
	if _, err := BuildReport(model); err == nil {
		t.Fatal("expected inconsistent anchor frame to be rejected")
	}
}
