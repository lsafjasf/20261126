package captionhub

import (
	"errors"
	"strconv"
	"time"
)

// Millis is a position on a caption timeline in milliseconds.
// The timeline is local to one session epoch and may restart at zero after
// a program switch or discontinuity.
type Millis int64

// At converts a duration to milliseconds, truncating sub-millisecond values.
func At(d time.Duration) Millis {
	return Millis(d / time.Millisecond)
}

// Duration returns the position as a time.Duration.
func (m Millis) Duration() time.Duration {
	return time.Duration(m) * time.Millisecond
}

// String renders the millisecond position as an integer with an "ms" suffix.
func (m Millis) String() string {
	return strconv.FormatInt(int64(m), 10) + "ms"
}

// Mode controls whether a connection follows live rotation or is pinned to a
// recovered replay position.
type Mode string

const (
	// ModeLive follows the current program segment and epoch.
	ModeLive Mode = "live"
	// ModeReplay stays pinned to the checkpoint's session/epoch.
	ModeReplay Mode = "replay"
)

// Status is the visible caption state for one connection.
type Status string

const (
	StatusNoSession        Status = "no_session"
	StatusAwaitingPosition Status = "awaiting_position"
	StatusIdle             Status = "idle"
	StatusVisible          Status = "visible"
	StatusReset            Status = "reset"
	StatusResyncRequired   Status = "resync_required"
)

// Stable reason codes used in ConnectionState.Reason.
const (
	ReasonConnected       = "connected"
	ReasonLateJoin        = "late_join"
	ReasonTimeUpdate      = "time_update"
	ReasonSeek            = "seek"
	ReasonSegmentStart    = "segment_start"
	ReasonProgramSwitch   = "program_switch"
	ReasonDiscontinuity   = "discontinuity"
	ReasonRecovered       = "position_recovered"
	ReasonLiveResync      = "live_resync"
	ReasonStaleCheckpoint = "stale_checkpoint"
	ReasonUnknownEpoch    = "unknown_epoch"
)

var (
	ErrNoSession           = errors.New("captionhub: no active caption session")
	ErrSessionNotFound     = errors.New("captionhub: caption session not found")
	ErrStaleSession        = errors.New("captionhub: caption session is sealed by a program switch")
	ErrEpochNotFound       = errors.New("captionhub: caption epoch not found")
	ErrStaleEpoch          = errors.New("captionhub: cue belongs to a stale session epoch")
	ErrStaleCheckpoint     = errors.New("captionhub: checkpoint session is no longer available")
	ErrAmbiguousCheckpoint = errors.New("captionhub: checkpoint requires session id or epoch")
	ErrInvalidCheckpoint   = errors.New("captionhub: invalid checkpoint")
	ErrConnectionNotFound  = errors.New("captionhub: connection not found")
	ErrConnectionExists    = errors.New("captionhub: connection already exists")
	ErrSegmentExists       = errors.New("captionhub: program segment already has a caption session")
	ErrSessionExists       = errors.New("captionhub: caption session id already exists")
	ErrSessionActive       = errors.New("captionhub: cannot forget the active live caption session")
	ErrDuplicateCue        = errors.New("captionhub: duplicate cue id")
	ErrInvalidCue          = errors.New("captionhub: invalid cue")
	ErrResyncRequired      = errors.New("captionhub: connection must resync to a caption session")
)

// Segment identifies one program segment and its caption session scope.
type Segment struct {
	ProgramID string `json:"program_id"`
	SegmentID string `json:"segment_id"`
	// SessionID is optional. When empty, Hub generates a stable unique id.
	SessionID string `json:"session_id,omitempty"`
}

func (s Segment) key() Segment {
	return Segment{ProgramID: s.ProgramID, SegmentID: s.SegmentID}
}

func (s Segment) valid() bool {
	return s.ProgramID != "" && s.SegmentID != ""
}

// Checkpoint is the token a viewer must persist for reconnect or seek recovery.
// MediaTime alone is intentionally insufficient after a timeline reset.
type Checkpoint struct {
	SessionID   string `json:"session_id"`
	Epoch       uint64 `json:"epoch"`
	ProgramID   string `json:"program_id,omitempty"`
	SegmentID   string `json:"segment_id,omitempty"`
	Mode        Mode   `json:"mode"`
	MediaTimeMS Millis `json:"media_time_ms,omitempty"`
}

// Cue is scoped to exactly one session epoch.
type Cue struct {
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
	Epoch     uint64 `json:"epoch"`
	Sequence  int64  `json:"sequence"`
	StartMS   Millis `json:"start_ms"`
	EndMS     Millis `json:"end_ms"`
	Text      string `json:"text"`
}

func (c Cue) visibleAt(pos Millis) bool {
	return pos >= c.StartMS && pos < c.EndMS
}

// SessionInfo is the public identity of the active caption timeline.
type SessionInfo struct {
	SessionID       string `json:"session_id"`
	ProgramID       string `json:"program_id"`
	SegmentID       string `json:"segment_id"`
	Epoch           uint64 `json:"epoch"`
	TimelineID      string `json:"timeline_id"`
	Reason          string `json:"reason"`
	Discontinuities int    `json:"discontinuities"`
}
