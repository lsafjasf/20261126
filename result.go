package ttmlevents

import "time"

// Event phases are intentionally ordered as retire -> enter -> change.
// Intervals are half-open [begin,end), so endings are applied before starts
// at the same timestamp when constructing the post-event snapshot.
const (
	PhaseEnd    = "end"
	PhaseBegin  = "begin"
	PhaseChange = "change"
)

// Event is one causally ordered row in the deterministic event table.
type Event struct {
	Sequence      int           `json:"sequence"`
	Time          time.Duration `json:"timeNanos"`
	TimeSeconds   float64       `json:"timeSeconds"`
	Timecode      string        `json:"timecode"`
	Phase         string        `json:"phase"`
	Kind          string        `json:"kind"`
	Target        string        `json:"target"`
	NodePath      string        `json:"nodePath,omitempty"`
	Title         string        `json:"title"`
	Detail        string        `json:"detail,omitempty"`
	CausedBy      string        `json:"causedBy,omitempty"`
	CauseSequence int           `json:"causeSequence,omitempty"`
	OrderKey      string        `json:"orderKey"`
	depth         int
	docOrder      int
	postOrder     int
	tie           int
}

// EventGroup contains every event at one timestamp in total causal order.
type EventGroup struct {
	Time            time.Duration `json:"timeNanos"`
	TimeSeconds     float64       `json:"timeSeconds"`
	Timecode        string        `json:"timecode"`
	Events          []Event       `json:"events"`
	SliceIndexAfter int           `json:"sliceIndexAfter"`
}

// TextRun is a source-ordered text node or explicit line break with its
// nearest timed ancestor's effective interval.
type TextRun struct {
	ID            string         `json:"id"`
	Kind          string         `json:"kind"`
	Text          string         `json:"text"`
	NodePath      string         `json:"nodePath,omitempty"`
	ElementPath   string         `json:"elementPath"`
	BlockPath     string         `json:"blockPath,omitempty"`
	Begin         time.Duration  `json:"beginNanos"`
	BeginSeconds  float64        `json:"beginSeconds"`
	End           *time.Duration `json:"endNanos,omitempty"`
	EndSeconds    *float64       `json:"endSeconds,omitempty"`
	TimingVisible bool           `json:"timingVisible"`
	Whitespace    bool           `json:"whitespace,omitempty"`
	Style         []Property     `json:"style,omitempty"`
}

// SliceRun is one run visible in a particular temporal slice.
type SliceRun struct {
	RunID    string     `json:"runId"`
	Text     string     `json:"text"`
	NodePath string     `json:"nodePath,omitempty"`
	Style    []Property `json:"style,omitempty"`
}

// Slice is an interval between two consecutive event boundaries. No source
// interval or keyframe boundary exists strictly inside a slice.
type Slice struct {
	Index        int            `json:"index"`
	Begin        time.Duration  `json:"beginNanos"`
	BeginSeconds float64        `json:"beginSeconds"`
	End          *time.Duration `json:"endNanos,omitempty"`
	EndSeconds   *float64       `json:"endSeconds,omitempty"`
	OpenEnded    bool           `json:"openEnded,omitempty"`
	Timecode     string         `json:"timecode"`
	Text         string         `json:"text"`
	Empty        bool           `json:"empty,omitempty"`
	Runs         []SliceRun     `json:"runs,omitempty"`
}

// StyleInterval is one effective property lane for one source text run.
type StyleInterval struct {
	RunID         string         `json:"runId"`
	Text          string         `json:"text,omitempty"`
	NodePath      string         `json:"nodePath,omitempty"`
	Property      string         `json:"property"`
	Begin         time.Duration  `json:"beginNanos"`
	BeginSeconds  float64        `json:"beginSeconds"`
	End           *time.Duration `json:"endNanos,omitempty"`
	EndSeconds    *float64       `json:"endSeconds,omitempty"`
	OpenEnded     bool           `json:"openEnded,omitempty"`
	Value         string         `json:"value"`
	EndValue      string         `json:"endValue,omitempty"`
	Interpolation string         `json:"interpolation"`
	Source        string         `json:"source"`
	AnimationPath string         `json:"animationPath,omitempty"`
	MergeGroup    string         `json:"-"`
}

// EventTable is the complete deterministic result of TTML event slicing.
type EventTable struct {
	OrderingRules    []string         `json:"orderingRules"`
	Warnings         []Warning        `json:"warnings,omitempty"`
	Timeline         *Timeline        `json:"timeline,omitempty"`
	Animations       []AnimationTrack `json:"animations,omitempty"`
	Runs             []TextRun        `json:"runs"`
	StyleIntervals   []StyleInterval  `json:"styleIntervals"`
	Slices           []Slice          `json:"slices"`
	Events           []Event          `json:"events"`
	Groups           []EventGroup     `json:"groups"`
	UniqueTimestamps int              `json:"uniqueTimestamps"`
}
