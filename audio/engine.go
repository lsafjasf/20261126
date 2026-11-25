package audio

import (
	"errors"
	"sync"
)

// State is the engine's negotiation/playback state.
type State int

const (
	// StateIdle: no device attached yet.
	StateIdle State = iota
	// StateRunning: stream parameters exactly match device capabilities.
	StateRunning
	// StateDegraded: running, but one or more parameters fell back to a
	// supported value (partial support).
	StateDegraded
	// StateDeviceLost: device removed or cannot carry the stream at all.
	// The sample position is frozen and preserved for recovery.
	StateDeviceLost
	// StateClosed: engine shut down.
	StateClosed
)

func (s State) String() string {
	switch s {
	case StateIdle:
		return "idle"
	case StateRunning:
		return "running"
	case StateDegraded:
		return "degraded"
	case StateDeviceLost:
		return "device_lost"
	case StateClosed:
		return "closed"
	default:
		return "unknown"
	}
}

// ErrAttached is returned when Attach is called twice.
var ErrAttached = errors.New("audio: engine already attached to a device")

// ErrNotRunning is returned by Render when no stream can consume frames.
var ErrNotRunning = errors.New("audio: engine is not in a running state")

// Engine owns the negotiation state between one player stream and one
// device. It is safe for concurrent use.
type Engine struct {
	mu      sync.Mutex
	state   State
	desired StreamParams
	neg     Negotiation
	pos     *Position
	dev     Device
	done    chan struct{}
	wg      sync.WaitGroup

	// OnChange, if set, is called (without the lock held) after every
	// state or negotiation change.
	OnChange func(state State, neg Negotiation)
}

func NewEngine(desired StreamParams) *Engine {
	return &Engine{
		state:   StateIdle,
		desired: desired,
		done:    make(chan struct{}),
	}
}

// Attach binds the engine to a device, performs the initial negotiation and
// starts watching device events.
func (e *Engine) Attach(dev Device) error {
	e.mu.Lock()
	if e.dev != nil {
		e.mu.Unlock()
		return ErrAttached
	}
	e.dev = dev
	e.mu.Unlock()

	e.renegotiate(dev.Capabilities())

	e.wg.Add(1)
	go e.loop(dev.Events())
	return nil
}

// State returns the current engine state.
func (e *Engine) State() State {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.state
}

// Negotiation returns the current negotiation result. The Selected params
// are the ones the render path must use — never the stale desired ones.
func (e *Engine) Negotiation() Negotiation {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.neg
}

// Position returns a consistent snapshot of the sample position.
func (e *Engine) Position() (sourceFrame, deviceFrame, totalDevice int64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.pos == nil {
		return 0, 0, 0
	}
	return e.pos.SourceFrame(), e.pos.DeviceFrames(), e.pos.TotalDeviceFrames()
}

// Anchors returns the recorded renegotiation anchors.
func (e *Engine) Anchors() []Anchor {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.pos == nil {
		return nil
	}
	return e.pos.Anchors()
}

// Render is called by the audio callback after n device-rate frames have
// been consumed by the hardware. It advances the sample position.
func (e *Engine) Render(frames int64) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.state != StateRunning && e.state != StateDegraded {
		return ErrNotRunning
	}
	e.pos.Advance(frames)
	return nil
}

// Close stops the engine and its event loop.
func (e *Engine) Close() {
	e.mu.Lock()
	if e.state == StateClosed {
		e.mu.Unlock()
		return
	}
	e.state = StateClosed
	close(e.done)
	e.mu.Unlock()
	e.wg.Wait()
}

func (e *Engine) loop(events <-chan DeviceEvent) {
	defer e.wg.Done()
	for {
		select {
		case <-e.done:
			return
		case ev, ok := <-events:
			if !ok {
				e.mu.Lock()
				if e.state != StateClosed {
					e.setStateLocked(StateDeviceLost)
				}
				e.mu.Unlock()
				e.notify()
				return
			}
			e.handleEvent(ev)
		}
	}
}

func (e *Engine) handleEvent(ev DeviceEvent) {
	switch ev.Kind {
	case EventRemoved:
		e.mu.Lock()
		// Freeze: pos keeps the exact sample position for recovery.
		e.setStateLocked(StateDeviceLost)
		e.mu.Unlock()
		e.notify()
	case EventCapsChanged, EventRestored:
		e.mu.Lock()
		dev := e.dev
		e.mu.Unlock()
		e.renegotiate(dev.Capabilities())
	}
}

// renegotiate matches desired params against fresh capabilities and, on
// success, retunes the position tracker so playback continues
// sample-continuously under the new device parameters.
func (e *Engine) renegotiate(caps Capabilities) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.state == StateClosed {
		return
	}
	neg, err := Negotiate(e.desired, caps)
	if err != nil {
		e.setStateLocked(StateDeviceLost)
		e.notifyLocked()
		return
	}
	changed := e.neg.Selected != neg.Selected
	e.neg = neg
	if e.pos == nil {
		e.pos = NewPosition(e.desired.SampleRate, neg.Selected.SampleRate, neg.Selected.Layout)
	} else if changed {
		e.pos.Retune(neg.Selected.SampleRate, neg.Selected.Layout)
	}
	if neg.Exact() {
		e.setStateLocked(StateRunning)
	} else {
		e.setStateLocked(StateDegraded)
	}
	e.notifyLocked()
}

func (e *Engine) setStateLocked(s State) { e.state = s }

func (e *Engine) notifyLocked() {
	cb := e.OnChange
	if cb == nil {
		return
	}
	state, neg := e.state, e.neg
	go cb(state, neg)
}

func (e *Engine) notify() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.notifyLocked()
}
