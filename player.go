package subselect

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Status is the playback transport state.
type Status string

const (
	StatusPaused  Status = "paused"
	StatusPlaying Status = "playing"
)

var (
	// ErrStaleSwitch is returned by a Switch call whose selection generation
	// was superseded by a newer Switch before it could commit. Its result
	// carries the currently committed view.
	ErrStaleSwitch = errors.New("subselect: stale switch generation discarded")

	// ErrNoAvailableTrack means the whole ordered fallback chain failed.
	ErrNoAvailableTrack = errors.New("subselect: no available subtitle track")
)

// View is an immutable, point-in-time snapshot of one browser session.
// Text identity (Decision.Track), rebased timing (Schedule) and transport
// position are always read from one consistent commit: an observer can never
// see a new language paired with an old track's time base.
type View struct {
	SessionID  string        `json:"session_id"`
	Generation uint64        `json:"generation"`
	Status     Status        `json:"status"`
	Position   time.Duration `json:"position_ns"`
	Off        bool          `json:"off"`

	Decision Decision  `json:"decision"`
	Schedule *Schedule `json:"schedule,omitempty"`
	Cue      *TimedCue `json:"cue,omitempty"`
}

// PositionMS returns the canonical position in milliseconds.
func (v *View) PositionMS() int64 {
	if v == nil {
		return 0
	}
	return v.Position.Milliseconds()
}

// Player is one browser/playback session. It owns all mutable selection and
// transport state; two browsers must never share a Player.
type Player struct {
	id       string
	selector *Selector
	loader   Loader
	clock    Clock

	mu     sync.Mutex
	status Status
	pos    time.Duration
	last   time.Time

	// gen is bumped by every Switch initiation. A load completion may only
	// commit when it still owns the current generation, which prevents an
	// older slow switch from overwriting a newer language selection.
	gen       uint64
	committed *View
}

// Option configures a Player.
type Option func(*Player)

// WithLoader supplies the cue loader.
func WithLoader(l Loader) Option {
	return func(p *Player) { p.loader = l }
}

// WithClock supplies the clock.
func WithClock(c Clock) Option {
	return func(p *Player) { p.clock = c }
}

// NewPlayer creates an isolated session in the paused state at position 0.
func NewPlayer(sessionID string, selector *Selector, opts ...Option) *Player {
	p := &Player{
		id:       sessionID,
		selector: selector,
		loader:   StaticLoader{},
		clock:    SystemClock{},
		status:   StatusPaused,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// ID returns the session id.
func (p *Player) ID() string { return p.id }

// SwitchResult is the outcome of a Switch attempt.
type SwitchResult struct {
	// View is the committed view (current view when Stale is true).
	View *View `json:"view"`

	// Decision is the resolver decision for this request.
	Decision Decision `json:"decision"`

	// LoadErrors records candidates that failed before a usable one.
	LoadErrors []TrackLoadError `json:"load_errors,omitempty"`

	// Stale marks a superseded switch whose result was not committed.
	Stale bool `json:"stale,omitempty"`
}

// Switch selects subtitles per criteria, walks the missing-track fallback
// chain when loads fail, rebases timings onto the canonical timeline and
// atomically commits the new view. Concurrent calls are safe; only the
// newest generation may commit.
func (p *Player) Switch(ctx context.Context, criteria Criteria) (SwitchResult, error) {
	p.mu.Lock()
	p.gen++
	gen := p.gen
	p.mu.Unlock()

	decision := p.selector.Select(criteria)

	var loadErrs []TrackLoadError
	var joined error

	for _, trackID := range decision.Candidates {
		if err := ctx.Err(); err != nil {
			p.mu.Lock()
			current := p.snapshotLocked(p.clock.Now())
			p.mu.Unlock()
			return SwitchResult{View: current, Decision: decision}, err
		}

		track, ok := p.selector.Catalog().Get(trackID)
		if !ok {
			err := errors.New("track vanished from catalog")
			loadErrs = append(loadErrs, TrackLoadError{TrackID: trackID, Err: err})
			joined = errors.Join(joined, TrackLoadError{TrackID: trackID, Err: err})
			continue
		}

		cues, err := p.loader.Load(ctx, track)
		if err != nil {
			loadErr := TrackLoadError{TrackID: trackID, Err: err}
			loadErrs = append(loadErrs, loadErr)
			joined = errors.Join(joined, loadErr)
			continue
		}

		schedule, err := Rebase(track, cues)
		if err != nil {
			loadErr := TrackLoadError{TrackID: trackID, Err: err}
			loadErrs = append(loadErrs, loadErr)
			joined = errors.Join(joined, loadErr)
			continue
		}

		p.mu.Lock()
		if p.gen != gen {
			current := p.snapshotLocked(p.clock.Now())
			p.mu.Unlock()
			return SwitchResult{
				View:       current,
				Decision:   decision,
				LoadErrors: loadErrs,
				Stale:      true,
			}, ErrStaleSwitch
		}

		p.advanceLocked(p.clock.Now())
		chosen := track
		finalDecision := decision
		finalDecision.Track = &chosen
		if len(loadErrs) > 0 {
			finalDecision.Reason = ReasonLoadFallback
		}
		view := &View{
			SessionID:  p.id,
			Generation: gen,
			Status:     p.status,
			Position:   p.pos,
			Decision:   finalDecision,
			Schedule:   schedule,
		}
		if cue, ok := schedule.Active(p.pos); ok {
			cueCopy := cue
			view.Cue = &cueCopy
		}
		p.committed = view
		result := SwitchResult{View: view, Decision: finalDecision, LoadErrors: loadErrs}
		p.mu.Unlock()
		return result, nil
	}

	// Exhausted (or empty) fallback chain: commit an explicit off view so
	// callers never keep a half-old/half-new presentation.
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.gen != gen {
		return SwitchResult{
			View:       p.snapshotLocked(p.clock.Now()),
			Decision:   decision,
			LoadErrors: loadErrs,
			Stale:      true,
		}, ErrStaleSwitch
	}

	p.advanceLocked(p.clock.Now())
	offDecision := decision
	offDecision.Track = nil
	offDecision.Reason = ReasonOff
	view := &View{
		SessionID:  p.id,
		Generation: gen,
		Status:     p.status,
		Position:   p.pos,
		Off:        true,
		Decision:   offDecision,
	}
	p.committed = view

	if joined == nil {
		joined = ErrNoAvailableTrack
	} else {
		joined = errors.Join(ErrNoAvailableTrack, joined)
	}
	return SwitchResult{View: view, Decision: offDecision, LoadErrors: loadErrs}, joined
}

// Play starts or resumes playback. It is idempotent.
func (p *Player) Play() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.advanceLocked(p.clock.Now())
	p.status = StatusPlaying
}

// Resume continues after Pause. Timing basis is reset to "now" without
// accumulating the paused interval.
func (p *Player) Resume() {
	p.Play()
}

// Pause freezes the canonical position and returns it.
func (p *Player) Pause() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.advanceLocked(p.clock.Now())
	p.status = StatusPaused
	return p.pos
}

// Seek moves the canonical position. Seeking while playing restarts the
// wall-clock basis so no time is credited for the jump itself.
func (p *Player) Seek(pos time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.advanceLocked(p.clock.Now())
	if pos < 0 {
		pos = 0
	}
	p.pos = pos
	p.last = p.clock.Now()
}

// Position returns the current canonical position.
func (p *Player) Position() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.advanceLocked(p.clock.Now())
	return p.pos
}

// Snapshot returns a consistent point-in-time view including the active cue.
func (p *Player) Snapshot() *View {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.snapshotLocked(p.clock.Now())
}

// advanceLocked credits elapsed clock time while playing. Callers hold p.mu.
func (p *Player) advanceLocked(now time.Time) {
	if !p.last.IsZero() && p.status == StatusPlaying {
		if delta := now.Sub(p.last); delta > 0 {
			p.pos += delta
		}
	}
	p.last = now
}

// snapshotLocked builds a current view from the last committed selection.
// Callers hold p.mu.
func (p *Player) snapshotLocked(now time.Time) *View {
	p.advanceLocked(now)

	if p.committed != nil {
		committed := *p.committed
		committed.Status = p.status
		committed.Position = p.pos
		committed.SessionID = p.id
		if committed.Schedule != nil {
			if cue, ok := committed.Schedule.Active(p.pos); ok {
				cueCopy := cue
				committed.Cue = &cueCopy
			} else {
				committed.Cue = nil
			}
		}
		return &committed
	}
	return &View{
		SessionID:  p.id,
		Generation: p.gen,
		Status:     p.status,
		Position:   p.pos,
	}
}
