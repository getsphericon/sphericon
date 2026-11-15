package testhelper

import (
	"sync"
	"time"
)

// Clock is a clock a test sets and moves by hand: the Instance reads it through
// WithClock(clock.Now), and a module a test builds takes clock.Now. It is safe for concurrent use,
// so writers racing on goroutines of their own read it while the test moves it.
type Clock struct {
	mu sync.Mutex
	at time.Time
}

// NewClock is a Clock standing at at.
func NewClock(at time.Time) *Clock { return &Clock{at: at} }

// Now is the time the clock stands at.
func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.at
}

// Set moves the clock to at.
func (c *Clock) Set(at time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.at = at
}

// Advance moves the clock forward by d.
func (c *Clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.at = c.at.Add(d)
}
