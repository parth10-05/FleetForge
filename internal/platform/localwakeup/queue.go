// Package localwakeup is a bounded deterministic, process-local wakeup adapter.
// Its contents are deliberately disposable: callers must rely on PostgreSQL-led
// repair after restart/loss, never on this queue as workflow truth.
package localwakeup

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/parth10-05/fleetforge/internal/application/orchestration"
)

var ErrClosed = errors.New("local wakeup queue closed")
var ErrOutage = errors.New("local wakeup queue unavailable")
var ErrAck = errors.New("local wakeup acknowledgement failed")

type Faults struct {
	Duplicate, Drop, Outage, OutcomeUnknown, AckFailure bool
	Delay                                               time.Duration
}
type Config struct {
	Capacity int
	Clock    orchestration.Clock
}
type Queue struct {
	mu       sync.Mutex
	capacity int
	clock    orchestration.Clock
	faults   Faults
	closed   bool
	sequence uint64
	items    []*item
}
type item struct {
	id           uint64
	raw          []byte
	available    time.Time
	acknowledged bool
}
type handle struct{ id uint64 }

func New(cfg Config) (*Queue, error) {
	if cfg.Capacity <= 0 || cfg.Capacity > 10000 || cfg.Clock == nil {
		return nil, fmt.Errorf("invalid local wakeup configuration")
	}
	return &Queue{capacity: cfg.Capacity, clock: cfg.Clock}, nil
}

// SetFaults is intentionally adapter-local and is intended only for deterministic
// tests/component fixtures; production composition must not expose it externally.
func (q *Queue) SetFaults(f Faults) { q.mu.Lock(); defer q.mu.Unlock(); q.faults = f }
func (q *Queue) Publish(_ context.Context, e orchestration.Envelope) (orchestration.PublishOutcome, error) {
	raw, err := orchestration.Encode(e)
	if err != nil {
		return orchestration.PublishRejectedPermanent, err
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return orchestration.PublishRejectedRetryable, ErrClosed
	}
	if q.faults.Outage {
		return orchestration.PublishRejectedRetryable, ErrOutage
	}
	if q.faults.Drop {
		return orchestration.PublishAccepted, nil
	}
	copies := 1
	if q.faults.Duplicate {
		copies = 2
	}
	if len(q.items)+copies > q.capacity {
		return orchestration.PublishRejectedRetryable, fmt.Errorf("local wakeup queue full")
	}
	for range copies {
		q.sequence++
		q.items = append(q.items, &item{id: q.sequence, raw: append([]byte(nil), raw...), available: q.clock.Now().Add(q.faults.Delay)})
	}
	if q.faults.OutcomeUnknown {
		return orchestration.PublishOutcomeUnknown, nil
	}
	return orchestration.PublishAccepted, nil
}
func (q *Queue) Receive(ctx context.Context, max int) ([]orchestration.Receipt, error) {
	if max <= 0 {
		return nil, fmt.Errorf("invalid receive batch")
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return nil, ErrClosed
	}
	if q.faults.Outage {
		return nil, ErrOutage
	}
	now := q.clock.Now()
	out := make([]orchestration.Receipt, 0, max)
	for _, it := range q.items {
		if len(out) == max {
			break
		}
		if !it.acknowledged && !it.available.After(now) {
			select {
			case <-ctx.Done():
				return out, ctx.Err()
			default:
			}
			out = append(out, orchestration.Receipt{Raw: append([]byte(nil), it.raw...), Handle: handle{id: it.id}})
		}
	}
	return out, nil
}
func (q *Queue) Ack(_ context.Context, r orchestration.Receipt) error {
	h, ok := r.Handle.(handle)
	if !ok {
		return fmt.Errorf("invalid local wakeup receipt")
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return ErrClosed
	}
	if q.faults.AckFailure {
		return ErrAck
	}
	for _, it := range q.items {
		if it.id == h.id {
			it.acknowledged = true
			return nil
		}
	}
	return fmt.Errorf("unknown local wakeup receipt")
}
func (q *Queue) Close() { q.mu.Lock(); defer q.mu.Unlock(); q.closed = true; q.items = nil }
