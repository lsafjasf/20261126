// Package anchor implements media-anchored event mapping.
//
// Events (chapters, forced subtitles and ordinary subtitles) are stored with
// intervals expressed in source-media time instead of absolute timeline time.
// When a film is re-cut, an edit decision list (EDL) describes where every
// source range lands on the new timeline. Mapping rebuilds every event against
// that EDL, splitting intervals that were cut into independently addressable
// pieces and flagging anything that cannot be mapped automatically for manual
// resolution.
package anchor

// Ticks is the integer time unit used everywhere. A project defines how many
// ticks make one second, so mapping never depends on floating point math.
type Ticks int64

// EventKind identifies the payload carried by an Event.
type EventKind string

const (
	KindChapter  EventKind = "chapter"
	KindForced   EventKind = "forced_subtitle"
	KindSubtitle EventKind = "subtitle"
)

// Media is a source asset. IDs are stable user-defined identifiers.
type Media struct {
	ID       string `json:"id"`
	Path     string `json:"path,omitempty"`
	Duration Ticks  `json:"duration_ticks"`
}

// Anchor binds an event interval to a range inside a source media file.
// This is the durable source of truth: timeline positions are derived from it.
type Anchor struct {
	MediaID string `json:"media_id"`
	Start   Ticks  `json:"start_ticks"`
	End     Ticks  `json:"end_ticks"`
}

// Event is a chapter or subtitle interval. Legacy events may have no anchor;
// they must be anchored (manually when the project contains several media)
// before they can participate in mapping.
type Event struct {
	ID     string    `json:"id"`
	Kind   EventKind `json:"kind"`
	Text   string    `json:"text,omitempty"`
	Anchor *Anchor   `json:"anchor,omitempty"`
}

// Segment is one EDL entry: a contiguous source range placed on the new
// timeline. The same source range may appear in several segments, which is
// exactly the case that needs a human choice.
type Segment struct {
	ID       string `json:"id"`
	MediaID  string `json:"media_id"`
	SrcStart Ticks  `json:"src_start_ticks"`
	SrcEnd   Ticks  `json:"src_end_ticks"`
	DstStart Ticks  `json:"dst_start_ticks"`
}

// DstEnd is the exclusive timeline end of the segment.
func (s Segment) DstEnd() Ticks {
	return s.DstStart + (s.SrcEnd - s.SrcStart)
}

// PieceStatus is the mapping state of one split interval.
type PieceStatus string

const (
	// StatusMapped resolved automatically or via a manual decision.
	StatusMapped PieceStatus = "mapped"
	// StatusAmbiguous matches multiple EDL segments and needs a choice.
	StatusAmbiguous PieceStatus = "ambiguous"
	// StatusRemoved has no surviving EDL segment (the footage was cut).
	StatusRemoved PieceStatus = "removed"
)

// Candidate is one selectable destination for an ambiguous piece.
type Candidate struct {
	SegmentID string `json:"segment_id"`
	DstStart  Ticks  `json:"dst_start_ticks"`
	DstEnd    Ticks  `json:"dst_end_ticks"`
}

// Piece is one fragment of an event after splitting at EDL cut boundaries.
type Piece struct {
	Key        string      `json:"key"`
	MediaID    string      `json:"media_id"`
	SrcStart   Ticks       `json:"src_start_ticks"`
	SrcEnd     Ticks       `json:"src_end_ticks"`
	Status     PieceStatus `json:"status"`
	SegmentID  string      `json:"segment_id,omitempty"`
	DstStart   Ticks       `json:"dst_start_ticks,omitempty"`
	DstEnd     Ticks       `json:"dst_end_ticks,omitempty"`
	Dropped    bool        `json:"dropped,omitempty"`
	Candidates []Candidate `json:"candidates,omitempty"`
}

// MappedEvent is the mapping output for one input event.
type MappedEvent struct {
	EventID string    `json:"event_id"`
	Kind    EventKind `json:"kind"`
	Pieces  []Piece   `json:"pieces"`
}

// Action is the manual resolution chosen for an unresolved piece.
type Action string

const (
	// ActionSegment maps the piece through a chosen EDL segment.
	ActionSegment Action = "segment"
	// ActionCustom places the piece at an explicit timeline interval.
	ActionCustom Action = "custom"
	// ActionDrop accepts that the footage was removed and drops the piece.
	ActionDrop Action = "drop"
)

// Decision is a durable manual resolution, keyed by the stable piece Key.
// No timeline timestamps are stored on the decision, so a decision stays
// meaningful even if later edits move the surrounding segments.
type Decision struct {
	Action    Action `json:"action"`
	SegmentID string `json:"segment_id,omitempty"`
	DstStart  Ticks  `json:"dst_start_ticks,omitempty"`
	DstEnd    Ticks  `json:"dst_end_ticks,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// AnchorDecision anchors a legacy event to a media interval manually.
type AnchorDecision struct {
	MediaID string `json:"media_id"`
	Start   Ticks  `json:"start_ticks"`
	End     Ticks  `json:"end_ticks"`
	Reason  string `json:"reason,omitempty"`
}

// Project is the single persisted document. All derived timeline data is
// rebuilt from Media, Events and Edl; Decisions only records human choices.
type Project struct {
	Version         int                       `json:"version"`
	TicksPerSecond  int64                     `json:"ticks_per_second"`
	Media           []Media                   `json:"media"`
	Events          []Event                   `json:"events"`
	Edl             []Segment                 `json:"edl"`
	Decisions       map[string]Decision       `json:"decisions,omitempty"`
	AnchorDecisions map[string]AnchorDecision `json:"anchor_decisions,omitempty"`
}
