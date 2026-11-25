package cea708

// Switcher is the service-selection facade: it demultiplexes CEA-708 service
// blocks, keeps one independent Decoder/Screen per service and performs the
// clear-and-latch transition when the viewer/program changes service.
//
// It is an alias of Manager so callers can use either name.
type Switcher = Manager

// New is the idiomatic entry point, equivalent to NewManager.
func New(opts ...Option) *Manager {
	return NewManager(opts...)
}

// NewSwitcher creates a service switcher.
func NewSwitcher(opts ...Option) *Switcher {
	return NewManager(opts...)
}
