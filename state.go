package trackstate

import "fmt"

// Flag identifies one independent track-state bit.
type Flag string

const (
	FlagMute    Flag = "mute"
	FlagSolo    Flag = "solo"
	FlagLock    Flag = "lock"
	FlagEnabled Flag = "enabled"
)

// State is the lossless representation of a track's four independent flags.
// In particular, solo, mute, and lock may all be true at the same time.
type State struct {
	Mute    bool `json:"mute"`
	Solo    bool `json:"solo"`
	Lock    bool `json:"lock"`
	Enabled bool `json:"enabled"`
}

// DefaultState is the state of a newly created, usable track.
func DefaultState() State {
	return State{Enabled: true}
}

// Equal reports whether two states contain the same flag values.
func (s State) Equal(other State) bool {
	return s == other
}

// Toggle changes exactly one flag and leaves the other three untouched.
func (s State) Toggle(flag Flag) (State, error) {
	switch flag {
	case FlagMute:
		s.Mute = !s.Mute
	case FlagSolo:
		s.Solo = !s.Solo
	case FlagLock:
		s.Lock = !s.Lock
	case FlagEnabled:
		s.Enabled = !s.Enabled
	default:
		return s, fmt.Errorf("unknown track flag %q", flag)
	}
	return s, nil
}

// With changes exactly one flag and leaves the other three untouched.
func (s State) With(flag Flag, value bool) (State, error) {
	switch flag {
	case FlagMute:
		s.Mute = value
	case FlagSolo:
		s.Solo = value
	case FlagLock:
		s.Lock = value
	case FlagEnabled:
		s.Enabled = value
	default:
		return s, fmt.Errorf("unknown track flag %q", flag)
	}
	return s, nil
}

// Status is the single-value state accepted by compact consumers.
type Status string

const (
	StatusActive   Status = "active"
	StatusSolo     Status = "solo"
	StatusMuted    Status = "muted"
	StatusLocked   Status = "locked"
	StatusDisabled Status = "disabled"
)

// ConflictPriority returns the documented precedence used when a state must
// be reduced to one Status. The first matching status wins.
func ConflictPriority() []Status {
	return []Status{
		StatusDisabled,
		StatusLocked,
		StatusMuted,
		StatusSolo,
		StatusActive,
	}
}

// Projection is a compact representation of a State.
//
// Status contains the selected single-value state. Unrepresentable lists the
// source flags that Status cannot carry and therefore must not be silently
// discarded when lossless data is still needed.
type Projection struct {
	Status          Status `json:"status"`
	Unrepresentable []Flag `json:"unrepresentable,omitempty"`
}

// Lossless reports whether Status carries every source flag.
func (p Projection) Lossless() bool {
	return len(p.Unrepresentable) == 0
}

// Project reduces a State using ConflictPriority.
//
// Disabled is the hard override. Lock comes next because losing edit
// protection is unsafe. Mute then wins over solo because an individual mute
// must not become audible merely because another flag is present. Solo follows
// mute, and active is the no-conflict fallback.
func (s State) Project() Projection {
	implied := DefaultState()
	status := StatusActive

	switch {
	case !s.Enabled:
		status = StatusDisabled
		implied.Enabled = false
	case s.Lock:
		status = StatusLocked
		implied.Lock = true
	case s.Mute:
		status = StatusMuted
		implied.Mute = true
	case s.Solo:
		status = StatusSolo
		implied.Solo = true
	}

	projection := Projection{Status: status}
	if s.Mute != implied.Mute {
		projection.Unrepresentable = append(projection.Unrepresentable, FlagMute)
	}
	if s.Solo != implied.Solo {
		projection.Unrepresentable = append(projection.Unrepresentable, FlagSolo)
	}
	if s.Lock != implied.Lock {
		projection.Unrepresentable = append(projection.Unrepresentable, FlagLock)
	}
	if s.Enabled != implied.Enabled {
		projection.Unrepresentable = append(projection.Unrepresentable, FlagEnabled)
	}
	return projection
}
