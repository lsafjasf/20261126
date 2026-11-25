package audio

import "sync"

// EventKind classifies device notifications.
type EventKind int

const (
	// EventCapsChanged means the device reconfigured itself (e.g. the
	// hardware switched from 44.1 kHz to 48 kHz) and Capabilities must be
	// re-read.
	EventCapsChanged EventKind = iota
	// EventRemoved means the device disappeared (unplugged, BT dropped).
	EventRemoved
	// EventRestored means a previously removed device came back.
	EventRestored
)

func (k EventKind) String() string {
	switch k {
	case EventCapsChanged:
		return "caps_changed"
	case EventRemoved:
		return "removed"
	case EventRestored:
		return "restored"
	default:
		return "unknown"
	}
}

// DeviceEvent is a notification from the device layer.
type DeviceEvent struct {
	Kind EventKind
}

// Device is the minimal hardware abstraction the engine negotiates with.
type Device interface {
	// Capabilities returns the device's current capabilities. It must be
	// safe to call after every DeviceEvent.
	Capabilities() Capabilities
	// Events delivers device notifications. The channel is closed when the
	// device is permanently gone and will never come back.
	Events() <-chan DeviceEvent
}

// SimDevice is an in-memory Device for tests and demos. Tests mutate it to
// emulate hardware switches, unplug and replug.
type SimDevice struct {
	mu     sync.Mutex
	caps   Capabilities
	events chan DeviceEvent
	closed bool
}

func NewSimDevice(caps Capabilities) *SimDevice {
	return &SimDevice{caps: caps, events: make(chan DeviceEvent, 16)}
}

func (d *SimDevice) Capabilities() Capabilities {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.caps
}

func (d *SimDevice) Events() <-chan DeviceEvent { return d.events }

// SetCaps changes the device capabilities and notifies listeners.
func (d *SimDevice) SetCaps(caps Capabilities) {
	d.mu.Lock()
	d.caps = caps
	d.mu.Unlock()
	d.emit(DeviceEvent{Kind: EventCapsChanged})
}

// Remove simulates the device disappearing.
func (d *SimDevice) Remove() { d.emit(DeviceEvent{Kind: EventRemoved}) }

// Restore simulates the device coming back, possibly with new capabilities.
func (d *SimDevice) Restore(caps Capabilities) {
	d.mu.Lock()
	d.caps = caps
	d.mu.Unlock()
	d.emit(DeviceEvent{Kind: EventRestored})
}

// Gone closes the event channel: the device will never return.
func (d *SimDevice) Gone() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.closed {
		d.closed = true
		close(d.events)
	}
}

func (d *SimDevice) emit(ev DeviceEvent) { d.events <- ev }
