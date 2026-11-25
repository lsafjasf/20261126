// Package scheduler places audio descriptions against a shared set of
// timeline constraints.
package scheduler

import "fmt"

const SchemaVersion = "1.0"

// Plan is the input document accepted by the scheduler.
type Plan struct {
	Timeline Timeline `json:"timeline"`
	Cues     []Cue    `json:"cues"`
	Options  Options  `json:"options,omitempty"`
}

// Timeline contains fixed occupancy and media metadata. All timestamps are
// milliseconds from the start of the media.
type Timeline struct {
	DurationMS        int64      `json:"duration_ms,omitempty"`
	Dialogue          []Interval `json:"dialogue,omitempty"`
	SoundEffects      []Interval `json:"sound_effects,omitempty"`
	Subtitles         []Interval `json:"subtitles,omitempty"`
	ExistingAudioDesc []Interval `json:"existing_audio_descriptions,omitempty"`
}

// Interval describes a fixed, occupied part of the timeline.
type Interval struct {
	ID      string `json:"id"`
	StartMS int64  `json:"start_ms"`
	EndMS   int64  `json:"end_ms"`
	Label   string `json:"label,omitempty"`
	Text    string `json:"text,omitempty"`
	Speaker string `json:"speaker,omitempty"`
}

// Cue is one piece of description that may be placed in the timeline. AnchorMS
// is the moment the description describes; it is used to rank valid slots.
type Cue struct {
	ID       string   `json:"id"`
	AnchorMS int64    `json:"anchor_ms"`
	Window   *Window  `json:"window,omitempty"`
	Priority int      `json:"priority,omitempty"`
	Long     *Variant `json:"long,omitempty"`
	Short    *Variant `json:"short,omitempty"`
}

// Window limits where a cue may be placed.
type Window struct {
	StartMS int64 `json:"start_ms"`
	EndMS   int64 `json:"end_ms"`
}

// Variant is a long or short rendition of a description. DurationMS takes
// precedence when set; otherwise the segment durations are summed.
type Variant struct {
	DurationMS int64     `json:"duration_ms,omitempty"`
	Segments   []Segment `json:"segments,omitempty"`
}

// Segment is an identifiable piece of description content. Segment IDs are
// matched between the long and short variants so omitted information can be
// reported precisely.
type Segment struct {
	ID         string `json:"id"`
	Text       string `json:"text"`
	DurationMS int64  `json:"duration_ms,omitempty"`
}

// Options controls default placement behavior. Zero values select defaults.
type Options struct {
	DefaultBeforeMS int64 `json:"default_before_ms,omitempty"`
	DefaultAfterMS  int64 `json:"default_after_ms,omitempty"`
	MinClearanceMS  int64 `json:"min_clearance_ms,omitempty"`
}

// ConstraintKind identifies the source of a forbidden interval.
type ConstraintKind string

const (
	ConstraintDialogue    ConstraintKind = "dialogue"
	ConstraintSoundEffect ConstraintKind = "sound_effect"
	ConstraintSubtitle    ConstraintKind = "subtitle"
	ConstraintAudioDesc   ConstraintKind = "audio_description"
)

// Constraint is a single merged occupancy interval. Constraints keep their
// original identity so conflict explanations can name what was avoided.
type Constraint struct {
	ID      string         `json:"id"`
	Kind    ConstraintKind `json:"kind"`
	StartMS int64          `json:"start_ms"`
	EndMS   int64          `json:"end_ms"`
	Label   string         `json:"label,omitempty"`
	CueID   string         `json:"cue_id,omitempty"`
	Variant string         `json:"variant,omitempty"`
}

// InfoDrop records content omitted from the placed version.
type InfoDrop struct {
	SegmentID string `json:"segment_id,omitempty"`
	Text      string `json:"text,omitempty"`
	Reason    string `json:"reason"`
}

// Conflict references a constraint that made a placement difficult or
// impossible.
type Conflict struct {
	ConstraintID string         `json:"constraint_id"`
	Kind         ConstraintKind `json:"kind"`
	StartMS      int64          `json:"start_ms"`
	EndMS        int64          `json:"end_ms"`
	Label        string         `json:"label,omitempty"`
	OverlapMS    int64          `json:"overlap_ms,omitempty"`
}

// Placement is a scheduled cue.
type Placement struct {
	CueID       string     `json:"cue_id"`
	Variant     string     `json:"variant"`
	StartMS     int64      `json:"start_ms"`
	EndMS       int64      `json:"end_ms"`
	AnchorMS    int64      `json:"anchor_ms"`
	DroppedInfo []InfoDrop `json:"dropped_info,omitempty"`
	Conflicts   []Conflict `json:"conflicts,omitempty"`
	Explanation string     `json:"explanation"`
}

// Omission is a cue for which neither variant could be placed.
type Omission struct {
	CueID       string     `json:"cue_id"`
	AnchorMS    int64      `json:"anchor_ms"`
	Reason      string     `json:"reason"`
	DroppedInfo []InfoDrop `json:"dropped_info,omitempty"`
	Conflicts   []Conflict `json:"conflicts,omitempty"`
	Explanation string     `json:"explanation"`
}

// Summary contains stable counters suitable for monitoring.
type Summary struct {
	Cues             int            `json:"cues"`
	LongPlaced       int            `json:"long_placed"`
	ShortPlaced      int            `json:"short_placed"`
	Omitted          int            `json:"omitted"`
	Constraints      int            `json:"constraints"`
	ConstraintCounts map[string]int `json:"constraint_counts"`
}

// Result is the scheduler output document.
type Result struct {
	SchemaVersion string       `json:"schema_version"`
	Placements    []Placement  `json:"placements"`
	Omitted       []Omission   `json:"omitted,omitempty"`
	Constraints   []Constraint `json:"constraints"`
	Summary       Summary      `json:"summary"`
}

func (r Interval) window() (Window, error) {
	if r.StartMS < 0 || r.EndMS < 0 {
		return Window{}, fmt.Errorf("%s: timestamps must be non-negative", r.ID)
	}
	if r.EndMS <= r.StartMS {
		return Window{}, fmt.Errorf("%s: end_ms must be greater than start_ms", r.ID)
	}
	return Window{StartMS: r.StartMS, EndMS: r.EndMS}, nil
}
