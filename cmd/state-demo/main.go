package main

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"

	"trackstate"
)

type stepResult struct {
	Action               string                     `json:"action"`
	Tracks               []trackstate.TrackSnapshot `json:"tracks"`
	OtherTracksUnchanged bool                       `json:"otherTracksUnchanged"`
}

type output struct {
	Initial trackstate.Snapshot `json:"initial"`
	Steps   []stepResult        `json:"steps"`
}

func main() {
	project := trackstate.Project{
		ID: "toggle-demo",
		Tracks: []trackstate.Track{
			{
				ID:       "edited",
				Name:     "Edited track",
				State:    trackstate.DefaultState(),
				Revision: 1,
			},
			{
				ID:       "left",
				Name:     "Unmodified mute track",
				State:    trackstate.State{Mute: true, Enabled: true},
				Revision: 7,
			},
			{
				ID:       "right",
				Name:     "Unmodified solo+lock track",
				State:    trackstate.State{Solo: true, Lock: true, Enabled: true},
				Revision: 11,
			},
		},
	}

	initial, err := trackstate.SnapshotProject(project)
	if err != nil {
		fail(err)
	}
	result := output{Initial: initial}

	toggles := []trackstate.Flag{
		trackstate.FlagMute,
		trackstate.FlagSolo,
		trackstate.FlagLock,
		trackstate.FlagEnabled,
	}
	for _, flag := range toggles {
		before, err := trackstate.SnapshotProject(project)
		if err != nil {
			fail(err)
		}
		edited := findTrack(project, "edited")
		toggled, err := edited.State.Toggle(flag)
		if err != nil {
			fail(err)
		}

		project, _, err = trackstate.ApplyPatch(project, trackstate.ImportPatch{
			ProjectID: project.ID,
			Tracks: []trackstate.TrackPatch{{
				TrackID: "edited",
				State:   oneFlagPatch(flag, toggled),
			}},
		})
		if err != nil {
			fail(err)
		}

		after, err := trackstate.SnapshotProject(project)
		if err != nil {
			fail(err)
		}
		result.Steps = append(result.Steps, stepResult{
			Action:               fmt.Sprintf("toggle %s on edited", flag),
			Tracks:               after.Tracks,
			OtherTracksUnchanged: otherTracksEqual(before, after),
		})
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		fail(err)
	}
}

func findTrack(project trackstate.Project, id string) trackstate.Track {
	for _, track := range project.Tracks {
		if track.ID == id {
			return track
		}
	}
	panic("missing demo track " + id)
}

func oneFlagPatch(flag trackstate.Flag, state trackstate.State) trackstate.StatePatch {
	switch flag {
	case trackstate.FlagMute:
		return trackstate.StatePatch{Mute: trackstate.BoolPtr(state.Mute)}
	case trackstate.FlagSolo:
		return trackstate.StatePatch{Solo: trackstate.BoolPtr(state.Solo)}
	case trackstate.FlagLock:
		return trackstate.StatePatch{Lock: trackstate.BoolPtr(state.Lock)}
	case trackstate.FlagEnabled:
		return trackstate.StatePatch{Enabled: trackstate.BoolPtr(state.Enabled)}
	default:
		panic("unknown demo flag")
	}
}

func otherTracksEqual(before, after trackstate.Snapshot) bool {
	if len(before.Tracks) != len(after.Tracks) {
		return false
	}
	for index := 1; index < len(before.Tracks); index++ {
		if !reflect.DeepEqual(before.Tracks[index], after.Tracks[index]) {
			return false
		}
	}
	return true
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "state-demo:", err)
	os.Exit(1)
}
