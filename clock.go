package subselect

import (
	"sync"
	"time"
)

// Clock abstracts wall-clock access so pause/resume acceptance tests stay
// deterministic and never sleep.
type Clock interface {
	Now() time.Time
}

// SystemClock uses the real wall clock.
type SystemClock struct{}

// Now implements Clock.
func (SystemClock) Now() time.Time { return time.Now() }

// FakeClock is a manually advanced, concurrency-safe clock.
type FakeClock struct {
	mu sync.RWMutex
	t  time.Time
}

// NewFakeClock creates a clock frozen at the given time.
func NewFakeClock(at time.Time) *FakeClock {
	return &FakeClock{t: at}
}

// Now implements Clock.
func (c *FakeClock) Now() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.t
}

// Advance moves the clock forward by d.
func (c *FakeClock) Advance(d time.Duration) time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
	return c.t
}

// Set replaces the current time.
func (c *FakeClock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = t
}
