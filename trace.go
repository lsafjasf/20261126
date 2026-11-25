package handlefix

// Source edge names used in trim traces.
const (
	EdgeSourceStart = "source_start"
	EdgeSourceEnd   = "source_end"
)

// EdgeEdit is one source-edge movement performed by a named pipeline step.
type EdgeEdit struct {
	Step   string `json:"step"`
	ClipID string `json:"clip_id"`
	Edge   string `json:"edge"`
	Before int64  `json:"before"`
	After  int64  `json:"after"`
}

// LostFrames reports how many frames an inward trim removed. Start trims lose
// frames when the edge moves right; end trims lose frames when it moves left.
func (e EdgeEdit) LostFrames() int64 {
	switch e.Edge {
	case EdgeSourceStart:
		return max64(0, e.After-e.Before)
	case EdgeSourceEnd:
		return max64(0, e.Before-e.After)
	default:
		return 0
	}
}

// DamageLocation ties a lost handle to a transition and the step that changed
// the matching source edge.
type DamageLocation struct {
	TransitionID string `json:"transition_id"`
	ClipID       string `json:"clip_id"`
	Side         string `json:"side"`
	Edge         string `json:"edge"`
	Step         string `json:"step"`
	LostFrames   int64  `json:"lost_frames"`
}

// DiffSourceEdges compares two project snapshots and records source edge
// movements under step. Sequence ranges are intentionally not copied here:
// callers compare those through Audit.
func DiffSourceEdges(step string, before, after Project) []EdgeEdit {
	beforeByID := make(map[string]Clip, len(before.Clips))
	for _, clip := range before.Clips {
		beforeByID[clip.ID] = clip
	}

	edits := make([]EdgeEdit, 0)
	for _, afterClip := range after.Clips {
		beforeClip, ok := beforeByID[afterClip.ID]
		if !ok {
			continue
		}
		if beforeClip.Source.Start != afterClip.Source.Start {
			edits = append(edits, EdgeEdit{
				Step: step, ClipID: afterClip.ID, Edge: EdgeSourceStart,
				Before: beforeClip.Source.Start, After: afterClip.Source.Start,
			})
		}
		if beforeClip.Source.End != afterClip.Source.End {
			edits = append(edits, EdgeEdit{
				Step: step, ClipID: afterClip.ID, Edge: EdgeSourceEnd,
				Before: beforeClip.Source.End, After: afterClip.Source.End,
			})
		}
	}
	return edits
}

// LocateDamage traverses every transition and associates inward source trims
// on its incoming start edge and outgoing end edge.
func LocateDamage(project Project, edits []EdgeEdit) []DamageLocation {
	locations := make([]DamageLocation, 0)
	for _, tr := range project.Transitions {
		for _, edit := range edits {
			lost := edit.LostFrames()
			if lost == 0 {
				continue
			}
			if edit.ClipID == tr.LeftClipID && edit.Edge == EdgeSourceEnd {
				locations = append(locations, DamageLocation{
					TransitionID: tr.ID,
					ClipID:       edit.ClipID,
					Side:         "outgoing_tail",
					Edge:         edit.Edge,
					Step:         edit.Step,
					LostFrames:   lost,
				})
			}
			if edit.ClipID == tr.RightClipID && edit.Edge == EdgeSourceStart {
				locations = append(locations, DamageLocation{
					TransitionID: tr.ID,
					ClipID:       edit.ClipID,
					Side:         "incoming_head",
					Edge:         edit.Edge,
					Step:         edit.Step,
					LostFrames:   lost,
				})
			}
		}
	}
	return locations
}
