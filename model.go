package sdh

import "time"

// Cue is one timed subtitle block as stored in SRT-like formats.
type Cue struct {
	// ID is the stable cue identifier; the SRT parser fills it with the
	// sequence number when present.
	ID string
	// Index is the 1-based sequence number used when rendering SRT.
	Index int
	// Start and End are the cue interval on the presentation timeline.
	Start time.Duration
	End   time.Duration
	// Text is the raw, possibly multi-line cue text.
	Text string
}

// Segment is one semantically classified piece of a cue.
type Segment struct {
	// Role is the semantic layer the segment belongs to.
	Role Role
	// Text is the cleaned segment text (wrapping markers removed).
	Text string
	// Raw is the original substring, including markers.
	Raw string
	// Line is the 0-based source line inside the cue.
	Line int
	// Speaker is the resolved speaker name when known.
	Speaker string
	// VoiceOver marks "(V.O.)"/"画外音" delivery.
	VoiceOver bool
	// OffScreen marks "(O.S.)"/"画外" delivery.
	OffScreen bool

	// pos is the byte offset used to keep segments in reading order.
	pos int
}

// Entry is a timed atom placed on a semantic layer. Entries inherit the
// interval of their source cue and carry the lane assigned by the overlap
// resolver.
type Entry struct {
	// ID is unique within a Project ("<cueID>#<segIndex>").
	ID string
	// CueID references the source Cue.ID.
	CueID string
	// CueIndex references the source Cue.Index.
	CueIndex int
	// SegIndex is the segment index inside the classified cue.
	SegIndex int

	Role      Role
	Text      string
	Speaker   string
	VoiceOver bool
	OffScreen bool

	Start time.Duration
	End   time.Duration

	// Group is the temporal overlap component id assigned by
	// ResolveOverlaps; -1 until layout has run.
	Group int
	// Lane is the non-colliding visual lane inside the role anchor band;
	// -1 until layout has run.
	Lane int
}

// Layer collects all entries of one semantic role in timeline order.
type Layer struct {
	Role    Role
	Entries []*Entry
}

// OverlapGroup is a connected component of temporally overlapping entries.
type OverlapGroup struct {
	ID       int
	Start    time.Duration
	End      time.Duration
	EntryIDs []string
	// MaxLane is the highest lane used inside the group across anchors.
	MaxLane int
}

// Layout is the result of overlap resolution.
type Layout struct {
	Groups []OverlapGroup
	// MaxLanes maps anchor to the number of lanes required there.
	MaxLanes map[Anchor]int
	// Overflow lists entry ids that exceed LayoutOptions.MaxLanes when that
	// cap is positive. Lanes are still assigned beyond the cap so output
	// stays complete; the report lets an editor flag the collision risk.
	Overflow []string
}

// Project is a parsed and split SDH timeline: source cues, classified
// segments, semantic layers and the flattened entry list.
type Project struct {
	Cues     []Cue
	Segments map[string][]Segment
	Layers   map[Role]*Layer
	// LayerOrder lists non-empty layers in editorial order.
	LayerOrder []Role
	Entries    []*Entry
	Layout     *Layout
}

// NewProject returns an initialized Project.
func NewProject() *Project {
	return &Project{
		Segments: map[string][]Segment{},
		Layers:   map[Role]*Layer{},
	}
}

// LayerEntries returns the entries for role, or nil when the layer is empty.
func (p *Project) LayerEntries(role Role) []*Entry {
	if l := p.Layers[role]; l != nil {
		return l.Entries
	}
	return nil
}
