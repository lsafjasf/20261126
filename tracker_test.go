package captionhub

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTrackerRejectsOldCueUntilReset(t *testing.T) {
	oldCP := Checkpoint{SessionID: "cs-1", Epoch: 1, Mode: ModeLive, MediaTimeMS: 3_000}
	tracker := NewTracker(oldCP)
	oldCue := Cue{ID: "old", SessionID: "cs-1", Epoch: 1, StartMS: 2_000, EndMS: 8_000, Text: "old"}
	if err := tracker.Accept(oldCue); err != nil {
		t.Fatal(err)
	}
	if visible := tracker.VisibleAt(3_000); len(visible) != 1 {
		t.Fatalf("visible old cues = %d, want 1", len(visible))
	}

	newCP := Checkpoint{SessionID: "cs-2", Epoch: 1, Mode: ModeLive}
	tracker.Reset(newCP)
	stale := oldCue
	stale.Text = "delayed old cue"
	err := tracker.Accept(stale)
	requireErrorIs(t, err, ErrStaleEpoch)
	if tracker.Rejected() != 1 {
		t.Fatalf("rejected = %d, want 1", tracker.Rejected())
	}

	newCue := Cue{ID: "new", SessionID: "cs-2", Epoch: 1, StartMS: 0, EndMS: 4_000, Text: "new"}
	if err := tracker.Accept(newCue); err != nil {
		t.Fatal(err)
	}
	if visible := tracker.VisibleAt(1_000); len(visible) != 1 || visible[0].ID != "new" {
		t.Fatalf("visible after reset = %+v, want new cue only", visible)
	}
}

func TestSnapshotJSONContainsEveryConnectionState(t *testing.T) {
	h := NewHub()
	mustSwitch(t, h, "SHOW", "seg")
	mustAddCue(t, h, Cue{ID: "c1", StartMS: 0, EndMS: 5_000, Text: "hello"})
	if _, err := h.ConnectAt("b", 1_000); err != nil {
		t.Fatal(err)
	}
	if _, err := h.ConnectAt("a", 2_000); err != nil {
		t.Fatal(err)
	}

	var builder strings.Builder
	if err := h.WriteStatesJSON(&builder); err != nil {
		t.Fatal(err)
	}
	jsonText := builder.String()
	if !json.Valid([]byte(jsonText)) {
		t.Fatalf("invalid JSON: %s", jsonText)
	}
	if strings.Index(jsonText, `"connection_id": "a"`) > strings.Index(jsonText, `"connection_id": "b"`) {
		t.Fatalf("connections are not sorted: %s", jsonText)
	}
	if !strings.Contains(jsonText, `"status": "visible"`) || !strings.Contains(jsonText, "hello") {
		t.Fatalf("JSON does not contain visible cue state: %s", jsonText)
	}
}
