package captionhub

import (
	"errors"
	"testing"
)

func mustSwitch(t *testing.T, h *Hub, program, segment string) SessionInfo {
	t.Helper()
	info, err := h.SwitchProgram(program, segment)
	if err != nil {
		t.Fatalf("SwitchProgram(%q,%q) error = %v", program, segment, err)
	}
	return info
}

func mustAddCue(t *testing.T, h *Hub, cue Cue) Cue {
	t.Helper()
	added, err := h.AddCue(cue)
	if err != nil {
		t.Fatalf("AddCue(%q) error = %v", cue.ID, err)
	}
	return added
}

func requireCueID(t *testing.T, state ConnectionState, want string) {
	t.Helper()
	if state.Status != StatusVisible {
		t.Fatalf("status = %q, want visible; state=%+v", state.Status, state)
	}
	if state.ActiveCue == nil {
		t.Fatalf("active cue is nil; visible=%+v", state.VisibleCues)
	}
	if state.ActiveCue.ID != want {
		t.Fatalf("active cue = %q, want %q", state.ActiveCue.ID, want)
	}
}

func requireNoCue(t *testing.T, state ConnectionState, want Status) {
	t.Helper()
	if state.Status != want {
		t.Fatalf("status = %q, want %q", state.Status, want)
	}
	if state.ActiveCue != nil || len(state.VisibleCues) != 0 {
		t.Fatalf("expected no visible cues, got %+v", state.VisibleCues)
	}
}

func requireErrorIs(t *testing.T, err error, target error) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error matching %v, got nil", target)
	}
	if !errors.Is(err, target) {
		t.Fatalf("error %v does not wrap %v", err, target)
	}
}
