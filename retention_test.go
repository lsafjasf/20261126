package captionhub

import "testing"

func TestForgetSessionInvalidatesPinnedReplay(t *testing.T) {
	h := NewHub()
	old := mustSwitch(t, h, "OLD", "old-seg")
	cue := mustAddCue(t, h, Cue{ID: "old-cue", StartMS: 0, EndMS: 4_000, Text: "old"})

	state, err := h.RecoverPosition("replay", Checkpoint{
		SessionID: old.SessionID, Epoch: 1, Mode: ModeReplay, MediaTimeMS: 1_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	requireCueID(t, state, cue.ID)

	current := mustSwitch(t, h, "NEW", "new-seg")
	if err := h.ForgetSession(old.SessionID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Cues(Checkpoint{SessionID: old.SessionID, Epoch: 1}); err == nil {
		t.Fatal("expected forgotten session cues to be unavailable")
	}

	forgotten, err := h.State("replay")
	if err != nil {
		t.Fatal(err)
	}
	requireNoCue(t, forgotten, StatusResyncRequired)
	if !forgotten.ResyncRequired {
		t.Fatal("pinned replay should require resync after session eviction")
	}

	err = h.ForgetSession(current.SessionID)
	requireErrorIs(t, err, ErrSessionActive)
}
