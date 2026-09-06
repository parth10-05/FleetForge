// Package lifecycle coordinates bounded, graceful process draining.
package lifecycle

import (
	"context"
	"sync"
)

// Controller stops new work before waiting for accepted local work to finish.
// It is process-local only; it is not a durable work lease or operation reservation.
type Controller struct {
	mu       sync.Mutex
	draining bool
	active   int
	changed  chan struct{}
}

func NewController() *Controller { return &Controller{changed: make(chan struct{})} }

// Acquire accepts work unless drain has begun. Its release function is idempotent.
func (c *Controller) Acquire() (release func(), accepted bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.draining {
		return func() {}, false
	}
	c.active++
	c.notifyLocked()
	var once sync.Once
	return func() { once.Do(func() { c.mu.Lock(); defer c.mu.Unlock(); c.active--; c.notifyLocked() }) }, true
}

func (c *Controller) BeginDrain() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.draining {
		return
	}
	c.draining = true
	c.notifyLocked()
}

// Drain rejects new work then waits for accepted work or the caller deadline.
func (c *Controller) Drain(ctx context.Context) bool {
	c.BeginDrain()
	for {
		c.mu.Lock()
		if c.active == 0 {
			c.mu.Unlock()
			return true
		}
		changed := c.changed
		c.mu.Unlock()
		select {
		case <-ctx.Done():
			return false
		case <-changed:
		}
	}
}

func (c *Controller) notifyLocked() { close(c.changed); c.changed = make(chan struct{}) }
