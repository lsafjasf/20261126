package captionhub

import (
	"errors"
	"testing"
)

func TestDiscontinuityChangesEpochAndPreservesOldEpochForReplay(t *testing.T) {
	h := NewHub()
	show := mustSwitch(t, h, "LIVE", "segment-1")
	before := mustAddCue(t, h, Cue{
		ID: "before-break", SessionID: show.SessionID, Epoch: show.Epoch,
		StartMS: 4_000, EndMS: 8_000, Text: "断点前",
	})

	live, err := h.ConnectAt("live-1", 5_000)
	if err != nil {
		t.Fatal(err)
	}
	requireCueID(t, live, before.ID)

	afterBreak, err := h.Discontinuity("EXT-X-DISCONTINUITY")
	if err != nil {
		t.Fatal(err)
	}
	if afterBreak.Epoch != 2 || afterBreak.Discontinuities != 1 {
		t.Fatalf("info = %+v, want epoch 2 and one discontinuity", afterBreak)
	}

	reset, err := h.State("live-1")
	if err != nil {
		t.Fatal(err)
	}
	requireNoCue(t, reset, StatusReset)
	if reset.Epoch != 2 || reset.TimelineID != show.SessionID+"#e2" {
		t.Fatalf("reset timeline = %s epoch %d", reset.TimelineID, reset.Epoch)
	}

	_, err = h.AddCue(before)
	requireErrorIs(t, err, ErrStaleEpoch)

	newCue := mustAddCue(t, h, Cue{
		ID: "after-break", SessionID: afterBreak.SessionID, Epoch: afterBreak.Epoch,
		StartMS: 1_000, EndMS: 5_000, Text: "信号恢复",
	})
	playing, err := h.ReportPosition("live-1", 2_000)
	if err != nil {
		t.Fatal(err)
	}
	requireCueID(t, playing, newCue.ID)

	replay, err := h.RecoverPosition("replay-old", Checkpoint{
		SessionID:   show.SessionID,
		Epoch:       show.Epoch,
		Mode:        ModeReplay,
		MediaTimeMS: 5_000,
	})
	if err != nil {
		t.Fatalf("recover old epoch: %v", err)
	}
	requireCueID(t, replay, before.ID)
	if replay.Mode != ModeReplay {
		t.Fatalf("mode = %q, want replay", replay.Mode)
	}
}

func TestRecoverPositionRestoresSeekAndHandlesMissingCheckpoint(t *testing.T) {
	h := NewHub()
	showA := mustSwitch(t, h, "SHOW-A", "a")
	cueA := mustAddCue(t, h, Cue{
		ID: "cue-a", StartMS: 10_000, EndMS: 14_000, Text: "A",
	})
	showB := mustSwitch(t, h, "SHOW-B", "b")
	cueB := mustAddCue(t, h, Cue{
		ID: "cue-b", SessionID: showB.SessionID, StartMS: 2_000, EndMS: 6_000, Text: "B",
	})

	recovered, err := h.RecoverPosition("viewer-replay", Checkpoint{
		SessionID: showA.SessionID, Epoch: 1, Mode: ModeReplay, MediaTimeMS: 11_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	requireCueID(t, recovered, cueA.ID)

	seeked, err := h.Seek("viewer-replay", 15_000)
	if err != nil {
		t.Fatal(err)
	}
	requireNoCue(t, seeked, StatusIdle)

	back, err := h.Seek("viewer-replay", 12_000)
	if err != nil {
		t.Fatal(err)
	}
	requireCueID(t, back, cueA.ID)

	missing, err := h.RecoverPosition("viewer-missing", Checkpoint{
		SessionID: "cs-gone", Epoch: 1, Mode: ModeReplay, MediaTimeMS: 3_000,
	})
	requireErrorIs(t, err, ErrSessionNotFound)
	requireNoCue(t, missing, StatusResyncRequired)
	if !missing.ResyncRequired || missing.Checkpoint == nil {
		t.Fatalf("missing checkpoint state should expose recovery token: %+v", missing)
	}
	if missing.Checkpoint.MediaTimeMS != 3_000 {
		t.Fatalf("attempted recovery time = %d, want 3000", missing.Checkpoint.MediaTimeMS)
	}
	persistedMissing, err := h.State("viewer-missing")
	if err != nil {
		t.Fatal(err)
	}
	if persistedMissing.Checkpoint == nil || persistedMissing.Checkpoint.MediaTimeMS != 3_000 {
		t.Fatalf("persisted checkpoint lost attempted time: %+v", persistedMissing.Checkpoint)
	}

	resynced, err := h.ResyncToLive("viewer-missing", 3_000)
	if err != nil {
		t.Fatal(err)
	}
	requireCueID(t, resynced, cueB.ID)
	if resynced.SessionID != showB.SessionID {
		t.Fatalf("resync session = %q, want %q", resynced.SessionID, showB.SessionID)
	}

	liveStale, err := h.RecoverPosition("viewer-live-stale", Checkpoint{
		SessionID: showA.SessionID, Epoch: 1, Mode: ModeLive, MediaTimeMS: 11_000,
	})
	if !errors.Is(err, ErrStaleCheckpoint) {
		t.Fatalf("live stale checkpoint error = %v, want ErrStaleCheckpoint", err)
	}
	requireNoCue(t, liveStale, StatusReset)
	if liveStale.SessionID != showB.SessionID {
		t.Fatalf("live recovery should point at current session, got %q", liveStale.SessionID)
	}
}

func TestCheckpointWithoutEpochIsAmbiguousAfterDiscontinuity(t *testing.T) {
	h := NewHub()
	show := mustSwitch(t, h, "LIVE", "seg")
	_, err := h.Discontinuity("")
	if err != nil {
		t.Fatal(err)
	}

	_, err = h.RecoverPosition("ambiguous", Checkpoint{
		SessionID: show.SessionID, Mode: ModeReplay, MediaTimeMS: 100,
	})
	requireErrorIs(t, err, ErrAmbiguousCheckpoint)

	state, stateErr := h.State("ambiguous")
	if stateErr != nil {
		t.Fatal(stateErr)
	}
	requireNoCue(t, state, StatusResyncRequired)
}
