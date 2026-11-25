package edlframe

const SchemaVersion = 1

// Document is the versioned projection of an EDL source list and event table.
type Document struct {
	SchemaVersion int           `json:"schemaVersion,omitempty"`
	Title         string        `json:"title,omitempty"`
	Reels         []ReelDecl    `json:"reels"`
	SourceList    []SourceEntry `json:"sourceList,omitempty"`
	Events        []Event       `json:"events"`
}

// ReelDecl contains the two reel-level frame-rate declarations that may disagree.
type ReelDecl struct {
	ReelID         string `json:"reelId"`
	SourceListRate string `json:"sourceListRate,omitempty"`
	EventTableRate string `json:"eventTableRate,omitempty"`
}

// SourceEntry is one row in the source list.
type SourceEntry struct {
	SourceID string `json:"sourceId"`
	ReelID   string `json:"reelId"`
	Rate     string `json:"rate"`
	Name     string `json:"name,omitempty"`
}

// Event is one EDL event. Rate overrides the reel's event-table rate.
type Event struct {
	Number                int    `json:"number,omitempty"`
	EventID               string `json:"eventId"`
	ReelID                string `json:"reelId"`
	SourceID              string `json:"sourceId,omitempty"`
	Rate                  string `json:"rate,omitempty"`
	SourceIn              string `json:"sourceIn"`
	SourceOut             string `json:"sourceOut"`
	RecordIn              string `json:"recordIn,omitempty"`
	RecordOut             string `json:"recordOut,omitempty"`
	DurationFrames        *int64 `json:"durationFrames,omitempty"`
	CrossesMidnight       bool   `json:"crossesMidnight,omitempty"`
	RecordCrossesMidnight bool   `json:"recordCrossesMidnight,omitempty"`
}

// Severity values used in reports.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
)

// Issue is a machine-readable validation or conflict finding.
type Issue struct {
	Code     string   `json:"code"`
	Severity Severity `json:"severity"`
	Field    string   `json:"field,omitempty"`
	Message  string   `json:"message"`
}

// IntervalReport describes one in/out pair in its declared timebase.
type IntervalReport struct {
	Field           string       `json:"field"`
	RawIn           string       `json:"rawIn"`
	RawOut          string       `json:"rawOut"`
	Timebase        string       `json:"timebase"`
	StartFrame      int64        `json:"startFrame"`
	EndFrame        int64        `json:"endFrame"`
	DurationFrames  int64        `json:"durationFrames"`
	EndDayOffset    int64        `json:"endDayOffset,omitempty"`
	CrossesMidnight bool         `json:"crossesMidnight,omitempty"`
	Valid           bool         `json:"valid"`
	Issues          []Issue      `json:"issues,omitempty"`
	Impact          *FrameImpact `json:"impact,omitempty"`
}

// FrameSpan is an exclusive frame interval [StartFrame, EndFrame).
type FrameSpan struct {
	StartFrame    int64  `json:"startFrame"`
	EndFrame      int64  `json:"endFrame"`
	FirstFrame    int64  `json:"firstFrame"`
	LastFrame     int64  `json:"lastFrame"`
	FrameCount    int64  `json:"frameCount"`
	StartTimecode string `json:"startTimecode"`
	EndTimecode   string `json:"endTimecode"`
	EndDayOffset  int64  `json:"endDayOffset,omitempty"`
}

// FrameSet identifies the exact canonical frames affected by an event.
type FrameSet struct {
	First     int64   `json:"first"`
	Last      int64   `json:"last"`
	Count     int64   `json:"count"`
	Truncated bool    `json:"truncated,omitempty"`
	Frames    []int64 `json:"frames,omitempty"`
}

// FrameImpact maps a declared interval into the unified timebase.
type FrameImpact struct {
	Policy             string    `json:"policy"`
	Declared           FrameSpan `json:"declared"`
	Canonical          FrameSpan `json:"canonical"`
	StartFrameDelta    int64     `json:"startFrameDelta"`
	EndFrameDelta      int64     `json:"endFrameDelta"`
	DurationFrameDelta int64     `json:"durationFrameDelta"`
	AffectedFrames     FrameSet  `json:"affectedFrames"`
}

// EventReport contains both rate-conflict and timecode-validation findings.
type EventReport struct {
	Index          int             `json:"index"`
	EventID        string          `json:"eventId"`
	Number         int             `json:"number,omitempty"`
	ReelID         string          `json:"reelId"`
	SourceID       string          `json:"sourceId,omitempty"`
	DeclaredRate   string          `json:"declaredRate,omitempty"`
	CanonicalRate  string          `json:"canonicalRate,omitempty"`
	RateConflict   bool            `json:"rateConflict"`
	Convertible    bool            `json:"convertible"`
	Issues         []Issue         `json:"issues,omitempty"`
	Source         *IntervalReport `json:"source,omitempty"`
	Record         *IntervalReport `json:"record,omitempty"`
	AffectedFrames *FrameSet       `json:"affectedFrames,omitempty"`
}

// ReelReport is the reel-level conflict plus all matching event findings.
type ReelReport struct {
	ReelID             string   `json:"reelId"`
	SourceListRate     string   `json:"sourceListRate,omitempty"`
	EventTableRate     string   `json:"eventTableRate,omitempty"`
	CanonicalRate      string   `json:"canonicalRate,omitempty"`
	RateConflict       bool     `json:"rateConflict"`
	Issues             []Issue  `json:"issues,omitempty"`
	AffectedEventIDs   []string `json:"affectedEventIds"`
	ConflictEventIDs   []string `json:"conflictEventIds"`
	InvalidEventIDs    []string `json:"invalidEventIds"`
	AffectedFrameCount int64    `json:"affectedFrameCount"`
}

// Summary gives stable counters for CLI exit status and tests.
type Summary struct {
	ReelCount           int   `json:"reelCount"`
	EventCount          int   `json:"eventCount"`
	ReelConflictCount   int   `json:"reelConflictCount"`
	EventConflictCount  int   `json:"eventConflictCount"`
	InvalidEventCount   int   `json:"invalidEventCount"`
	ConvertedEventCount int   `json:"convertedEventCount"`
	AffectedFrameCount  int64 `json:"affectedFrameCount"`
}

// Report is the complete detection result.
type Report struct {
	SchemaVersion int           `json:"schemaVersion"`
	Policy        UnifyOptions  `json:"policy"`
	Reels         []ReelReport  `json:"reels"`
	Events        []EventReport `json:"events"`
	Summary       Summary       `json:"summary"`
	Issues        []Issue       `json:"issues,omitempty"`
}

// UnifyOptions controls canonical rate selection and report expansion.
type UnifyOptions struct {
	CanonicalRate string `json:"canonicalRate,omitempty"`
	Prefer        string `json:"prefer,omitempty"` // "source-list" (default) or "event-table"
	Policy        string `json:"policy,omitempty"` // "preserve-wall-clock" (default)
	ExpandFrames  bool   `json:"expandFrames,omitempty"`
}

func (o UnifyOptions) withDefaults() UnifyOptions {
	if o.Prefer == "" {
		o.Prefer = "source-list"
	}
	if o.Policy == "" {
		o.Policy = "preserve-wall-clock"
	}
	return o
}
