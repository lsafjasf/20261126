package trackstate

import (
	"reflect"
	"slices"
	"testing"
)

func TestProjectionMatrix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		state      State
		wantStatus Status
		wantLost   []Flag
	}{
		{
			name:       "active",
			state:      State{Enabled: true},
			wantStatus: StatusActive,
		},
		{
			name:       "muted",
			state:      State{Mute: true, Enabled: true},
			wantStatus: StatusMuted,
		},
		{
			name:       "solo",
			state:      State{Solo: true, Enabled: true},
			wantStatus: StatusSolo,
		},
		{
			name:       "locked",
			state:      State{Lock: true, Enabled: true},
			wantStatus: StatusLocked,
		},
		{
			name:       "disabled",
			state:      State{Enabled: false},
			wantStatus: StatusDisabled,
		},
		{
			name:       "mute and solo",
			state:      State{Mute: true, Solo: true, Enabled: true},
			wantStatus: StatusMuted,
			wantLost:   []Flag{FlagSolo},
		},
		{
			name:       "lock beats mute and solo",
			state:      State{Mute: true, Solo: true, Lock: true, Enabled: true},
			wantStatus: StatusLocked,
			wantLost:   []Flag{FlagMute, FlagSolo},
		},
		{
			name:       "disabled beats every other flag",
			state:      State{Mute: true, Solo: true, Lock: true, Enabled: false},
			wantStatus: StatusDisabled,
			wantLost:   []Flag{FlagMute, FlagSolo, FlagLock},
		},
		{
			name:       "disabled with mute",
			state:      State{Mute: true, Enabled: false},
			wantStatus: StatusDisabled,
			wantLost:   []Flag{FlagMute},
		},
		{
			name:       "lock and mute",
			state:      State{Mute: true, Lock: true, Enabled: true},
			wantStatus: StatusLocked,
			wantLost:   []Flag{FlagMute},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := test.state.Project()
			if got.Status != test.wantStatus {
				t.Fatalf("status = %q, want %q", got.Status, test.wantStatus)
			}
			if !slices.Equal(got.Unrepresentable, test.wantLost) {
				t.Fatalf(
					"unrepresentable = %v, want %v",
					got.Unrepresentable,
					test.wantLost,
				)
			}
			if got.Lossless() != (len(test.wantLost) == 0) {
				t.Fatalf("Lossless() disagrees with lost flags %v", test.wantLost)
			}
		})
	}
}

func TestAllSixteenProjectionCombinations(t *testing.T) {
	t.Parallel()

	wantStatuses := []Status{
		StatusActive,
		StatusMuted,
		StatusSolo,
		StatusMuted,
		StatusLocked,
		StatusLocked,
		StatusLocked,
		StatusLocked,
		StatusDisabled,
		StatusDisabled,
		StatusDisabled,
		StatusDisabled,
		StatusDisabled,
		StatusDisabled,
		StatusDisabled,
		StatusDisabled,
	}
	wantLost := [][]Flag{
		nil,
		nil,
		nil,
		{FlagSolo},
		nil,
		{FlagMute},
		{FlagSolo},
		{FlagMute, FlagSolo},
		nil,
		{FlagMute},
		{FlagSolo},
		{FlagMute, FlagSolo},
		{FlagLock},
		{FlagMute, FlagLock},
		{FlagSolo, FlagLock},
		{FlagMute, FlagSolo, FlagLock},
	}

	lossless := 0
	for mask := 0; mask < 16; mask++ {
		state := State{
			Mute:    mask&1 != 0,
			Solo:    mask&2 != 0,
			Lock:    mask&4 != 0,
			Enabled: mask < 8,
		}
		got := state.Project()
		if got.Status != wantStatuses[mask] {
			t.Fatalf(
				"mask %04b status = %q, want %q",
				mask,
				got.Status,
				wantStatuses[mask],
			)
		}
		if !slices.Equal(got.Unrepresentable, wantLost[mask]) {
			t.Fatalf(
				"mask %04b unrepresentable = %v, want %v",
				mask,
				got.Unrepresentable,
				wantLost[mask],
			)
		}
		if got.Lossless() {
			lossless++
		}
	}
	if lossless != 5 {
		t.Fatalf("lossless combinations = %d, want 5", lossless)
	}
}

func TestConflictPriorityOrder(t *testing.T) {
	t.Parallel()

	want := []Status{
		StatusDisabled,
		StatusLocked,
		StatusMuted,
		StatusSolo,
		StatusActive,
	}
	if got := ConflictPriority(); !slices.Equal(got, want) {
		t.Fatalf("priority = %v, want %v", got, want)
	}
}

func TestSequentialTogglesKeepOtherTracksUntouched(t *testing.T) {
	t.Parallel()

	project := Project{
		ID: "sequence-demo",
		Tracks: []Track{
			{ID: "edited", State: DefaultState(), Revision: 1},
			{ID: "left", State: State{Mute: true, Enabled: true}, Revision: 7},
			{ID: "right", State: State{Solo: true, Lock: true, Enabled: true}, Revision: 11},
		},
	}

	steps := []struct {
		flag       Flag
		want       State
		wantStatus Status
		wantLost   []Flag
	}{
		{
			flag:       FlagMute,
			want:       State{Mute: true, Enabled: true},
			wantStatus: StatusMuted,
		},
		{
			flag:       FlagSolo,
			want:       State{Mute: true, Solo: true, Enabled: true},
			wantStatus: StatusMuted,
			wantLost:   []Flag{FlagSolo},
		},
		{
			flag:       FlagLock,
			want:       State{Mute: true, Solo: true, Lock: true, Enabled: true},
			wantStatus: StatusLocked,
			wantLost:   []Flag{FlagMute, FlagSolo},
		},
		{
			flag:       FlagEnabled,
			want:       State{Mute: true, Solo: true, Lock: true, Enabled: false},
			wantStatus: StatusDisabled,
			wantLost:   []Flag{FlagMute, FlagSolo, FlagLock},
		},
	}

	for stepIndex, step := range steps {
		before, err := SnapshotProject(project)
		if err != nil {
			t.Fatal(err)
		}
		current := before.Tracks[0].State
		toggled, err := current.Toggle(step.flag)
		if err != nil {
			t.Fatal(err)
		}

		project, _, err = ApplyPatch(project, ImportPatch{
			ProjectID: project.ID,
			Tracks: []TrackPatch{{
				TrackID: "edited",
				State:   patchOneFlag(step.flag, toggled),
			}},
		})
		if err != nil {
			t.Fatalf("step %d: %v", stepIndex, err)
		}

		after, err := SnapshotProject(project)
		if err != nil {
			t.Fatal(err)
		}
		if !after.Tracks[0].State.Equal(step.want) {
			t.Fatalf(
				"step %d state = %+v, want %+v",
				stepIndex,
				after.Tracks[0].State,
				step.want,
			)
		}
		if after.Tracks[0].ProjectedStatus != step.wantStatus {
			t.Fatalf(
				"step %d status = %q, want %q",
				stepIndex,
				after.Tracks[0].ProjectedStatus,
				step.wantStatus,
			)
		}
		if !slices.Equal(after.Tracks[0].Unrepresentable, step.wantLost) {
			t.Fatalf(
				"step %d lost = %v, want %v",
				stepIndex,
				after.Tracks[0].Unrepresentable,
				step.wantLost,
			)
		}

		for index := 1; index < len(before.Tracks); index++ {
			if !reflect.DeepEqual(before.Tracks[index], after.Tracks[index]) {
				t.Fatalf(
					"step %d unexpectedly changed track %q: before=%+v after=%+v",
					stepIndex,
					before.Tracks[index].TrackID,
					before.Tracks[index],
					after.Tracks[index],
				)
			}
		}
	}
}

func patchOneFlag(flag Flag, state State) StatePatch {
	var value bool
	switch flag {
	case FlagMute:
		value = state.Mute
		return StatePatch{Mute: BoolPtr(value)}
	case FlagSolo:
		value = state.Solo
		return StatePatch{Solo: BoolPtr(value)}
	case FlagLock:
		value = state.Lock
		return StatePatch{Lock: BoolPtr(value)}
	case FlagEnabled:
		value = state.Enabled
		return StatePatch{Enabled: BoolPtr(value)}
	default:
		panic("unknown flag")
	}
}
