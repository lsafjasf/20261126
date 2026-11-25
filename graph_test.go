package eac3

import (
	"errors"
	"strings"
	"testing"
)

func reasonCodes(sw Switch) map[string]bool {
	out := make(map[string]bool, len(sw.Reasons))
	for _, r := range sw.Reasons {
		out[r.Code] = true
	}
	return out
}

func eventKinds(sw Switch) map[EventKind]bool {
	out := make(map[EventKind]bool, len(sw.Events))
	for _, ev := range sw.Events {
		out[ev.Kind] = true
	}
	return out
}

func TestDemoSwitchGraphReasonsAndScopedEvents(t *testing.T) {
	g, switches, err := RunDemo()
	if err != nil {
		t.Fatal(err)
	}
	if len(switches) != 5 {
		t.Fatalf("switches=%d", len(switches))
	}

	want := [][]string{
		{ReasonInitialPrepare},
		{ReasonModeChanged, ReasonSubstreamChanged, ReasonChannelChanged, ReasonFrameCadenceChanged, ReasonCodedBoundaryChanged},
		{ReasonModeChanged, ReasonSubstreamChanged, ReasonDRCChanged},
		{ReasonProgramChanged, ReasonSampleRateChanged, ReasonChannelChanged, ReasonDRCChanged, ReasonFrameCadenceChanged},
		{ReasonNoChange},
	}
	for i, codes := range want {
		got := reasonCodes(switches[i])
		for _, code := range codes {
			if !got[code] {
				t.Fatalf("switch %d missing reason %s; got %+v", i+1, code, got)
			}
		}
	}

	associated := switches[1]
	kinds := eventKinds(associated)
	for _, kind := range []EventKind{EventBoundaryGate, EventFrameResync, EventProgramGraph, EventMixer, EventOutput} {
		if !kinds[kind] {
			t.Fatalf("associated switch missing event %s: %+v", kind, kinds)
		}
	}
	if kinds[EventDecoder] {
		t.Fatalf("same sample rate should not force decoder rebuild: %+v", associated.Events)
	}
	if !associated.Boundary.WaitForSyncword || !associated.Boundary.ResetCadence {
		t.Fatalf("boundary decision not controlled: %+v", associated.Boundary)
	}

	mixed := switches[2]
	mixedKinds := eventKinds(mixed)
	if !mixedKinds[EventDRC] || !mixedKinds[EventProgramGraph] {
		t.Fatalf("mixed DRC switch missing scoped events: %+v", mixed.Events)
	}
	if mixedKinds[EventOutput] || mixedKinds[EventDecoder] {
		t.Fatalf("DRC-only change should not rebuild decoder/output: %+v", mixed.Events)
	}

	cross := switches[3]
	crossKinds := eventKinds(cross)
	for _, kind := range []EventKind{EventDecoder, EventOutput, EventMixer, EventDRC, EventFrameResync} {
		if !crossKinds[kind] {
			t.Fatalf("cross-program switch missing %s", kind)
		}
	}

	noop := switches[4]
	if !noop.NoOp || len(noop.Events) != 0 {
		t.Fatalf("repeat selection should be noop without events: %+v", noop)
	}

	for _, sw := range switches[:4] {
		if len(sw.Reasons) == 0 {
			t.Fatalf("switch #%d has no reason", sw.Sequence)
		}
		for i, ev := range sw.Events {
			if ev.Order != i+1 {
				t.Fatalf("event order gap in #%d: index=%d order=%d", sw.Sequence, i, ev.Order)
			}
			if len(ev.Reasons) == 0 || len(ev.Actions) == 0 {
				t.Fatalf("event %s in #%d must carry reasons and actions", ev.Kind, sw.Sequence)
			}
		}
	}

	if g.CurrentID != switches[3].ToID {
		t.Fatalf("current=%s want %s", g.CurrentID, switches[3].ToID)
	}
	if !strings.Contains(g.Mermaid(), "flowchart LR") || !strings.Contains(g.Markdown(), "受控重建事件") {
		t.Fatal("report output missing expected content")
	}
}

func TestDependentStateRequiresObservedAnchor(t *testing.T) {
	g := NewGraph()
	err := g.Observe(FrameInfo{
		ProgramID: "p", Type: SubstreamDependent, SubstreamID: 0,
		Spec: ProgramSpec{
			SampleRateHz: 48000,
			Channel:      ChannelStructure{Channels: 8},
			Boundary:     BoundaryState{SamplesPerFrame: 768},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	anchor := SubstreamRef{Type: SubstreamIndependent, ID: 0}
	_, err = g.Declare(State{
		ProgramID: "p", Mode: ModeAssociated, Anchor: &anchor,
		Spec: ProgramSpec{
			SampleRateHz: 48000,
			Channel:      ChannelStructure{Channels: 8},
			Boundary:     BoundaryState{SamplesPerFrame: 768},
		},
	})
	if !errors.Is(err, ErrAnchorMissing) {
		t.Fatalf("error=%v want ErrAnchorMissing", err)
	}
}

func TestForceResyncSameStateEmitsResyncEvent(t *testing.T) {
	g, requests, err := DemoGraph()
	if err != nil {
		t.Fatal(err)
	}
	first, err := g.Switch(requests[0])
	if err != nil {
		t.Fatal(err)
	}
	req := requests[0]
	req.ForceResync = true
	req.Reason = "检测到码流 gap"
	second, err := g.Switch(req)
	if err != nil {
		t.Fatal(err)
	}
	if second.NoOp {
		t.Fatal("force resync must not be noop")
	}
	if !reasonCodes(second)[ReasonResyncRequired] {
		t.Fatalf("missing resync reason: %+v", second.Reasons)
	}
	if !eventKinds(second)[EventFrameResync] {
		t.Fatalf("missing frame resync event: %+v", second.Events)
	}
	if !second.Boundary.DropPartialBytes {
		t.Fatal("force resync should drop partial bytes")
	}
	if first.Sequence != 1 || second.Sequence != 2 {
		t.Fatalf("sequences=%d,%d", first.Sequence, second.Sequence)
	}
}
