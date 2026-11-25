package captionhub

import "testing"

func TestProgramSwitchSealsOldSessionAndClearsLiveConnections(t *testing.T) {
	h := NewHub(WithIDPrefix("test"))
	showA := mustSwitch(t, h, "NEWS", "news-1")
	cueA := mustAddCue(t, h, Cue{
		ID: "news-opening", SessionID: showA.SessionID,
		StartMS: 2_000, EndMS: 8_000, Text: "晚上好",
	})

	live, err := h.ConnectAt("conn-live", 3_000)
	if err != nil {
		t.Fatal(err)
	}
	requireCueID(t, live, cueA.ID)

	late, err := h.ConnectAt("conn-late", 5_000)
	if err != nil {
		t.Fatal(err)
	}
	requireCueID(t, late, cueA.ID)

	expired, err := h.ConnectAt("conn-expired", 9_000)
	if err != nil {
		t.Fatal(err)
	}
	requireNoCue(t, expired, StatusIdle)

	showB := mustSwitch(t, h, "DRAMA", "drama-1")
	if showB.SessionID == showA.SessionID || showB.Epoch != 1 {
		t.Fatalf("new session not started: A=%+v B=%+v", showA, showB)
	}

	reset, err := h.State("conn-live")
	if err != nil {
		t.Fatal(err)
	}
	requireNoCue(t, reset, StatusReset)
	if reset.SessionID != showB.SessionID || reset.Epoch != 1 {
		t.Fatalf("connection bound to %s#%d, want %s#1", reset.SessionID, reset.Epoch, showB.SessionID)
	}
	if reset.MediaTimeMS != nil {
		t.Fatalf("reset connection retained media time %d", *reset.MediaTimeMS)
	}

	_, err = h.AddCue(cueA)
	requireErrorIs(t, err, ErrStaleSession)

	staleDelivery, err := h.DeliverCue("conn-live", cueA)
	requireErrorIs(t, err, ErrStaleSession)
	if staleDelivery.RejectedCues != 1 {
		t.Fatalf("rejected = %d, want 1", staleDelivery.RejectedCues)
	}
	if staleDelivery.ResyncRequired {
		t.Fatal("a healthy current connection should not require resync after discarding one stale cue")
	}
	requireNoCue(t, staleDelivery, StatusReset)

	cueB := mustAddCue(t, h, Cue{
		ID: "drama-opening", SessionID: showB.SessionID,
		StartMS: 0, EndMS: 4_000, Text: "第二集",
	})
	playingB, err := h.ReportPosition("conn-live", 1_000)
	if err != nil {
		t.Fatal(err)
	}
	requireCueID(t, playingB, cueB.ID)
	if playingB.Checkpoint == nil || playingB.Checkpoint.SessionID != showB.SessionID {
		t.Fatalf("checkpoint not refreshed: %+v", playingB.Checkpoint)
	}

	states := h.States()
	if len(states) != 3 {
		t.Fatalf("States() len = %d, want 3", len(states))
	}
	for _, state := range states {
		if state.ConnectionID == "conn-live" {
			requireCueID(t, state, cueB.ID)
		}
		for _, cue := range state.VisibleCues {
			if cue.SessionID == showA.SessionID {
				t.Fatalf("connection %s still shows old session cue %s", state.ConnectionID, cue.ID)
			}
		}
	}
}

func TestLateViewerDoesNotReplayExpiredCues(t *testing.T) {
	h := NewHub()
	show := mustSwitch(t, h, "GAME", "q4")
	mustAddCue(t, h, Cue{ID: "expired", StartMS: 0, EndMS: 1_000, Text: "old"})
	active := mustAddCue(t, h, Cue{ID: "active", StartMS: 4_000, EndMS: 9_000, Text: "now"})
	mustAddCue(t, h, Cue{ID: "upcoming", StartMS: 10_000, EndMS: 12_000, Text: "soon"})

	state, err := h.ConnectAt("late", 5_000)
	if err != nil {
		t.Fatal(err)
	}
	requireCueID(t, state, active.ID)
	if len(state.VisibleCues) != 1 {
		t.Fatalf("visible cues = %d, want only active cue", len(state.VisibleCues))
	}
	if state.SessionID != show.SessionID {
		t.Fatalf("session = %q, want %q", state.SessionID, show.SessionID)
	}
}
