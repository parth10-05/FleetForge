// Package clock supplies logical-time primitives for deterministic simulation.
package clock

import (
	"errors"
	"sort"
)

var ErrTimeReversal = errors.New("logical time cannot move backwards")

type LogicalTime int64

type Event struct {
	Due      LogicalTime
	Priority int
	TargetID string
	Sequence uint64
	Value    string
}

// Scheduler orders timers by due time, version-fixed priority, target ID, then
// insertion sequence. It contains no wall-clock or goroutine behavior.
type Scheduler struct {
	next   uint64
	events []Event
}

func (s *Scheduler) Schedule(due LogicalTime, priority int, targetID, value string) Event {
	s.next++
	event := Event{Due: due, Priority: priority, TargetID: targetID, Sequence: s.next, Value: value}
	s.events = append(s.events, event)
	sort.SliceStable(s.events, func(i, j int) bool { return before(s.events[i], s.events[j]) })
	return event
}

func (s *Scheduler) PopDue(now LogicalTime) []Event {
	cutoff := 0
	for cutoff < len(s.events) && s.events[cutoff].Due <= now {
		cutoff++
	}
	due := append([]Event(nil), s.events[:cutoff]...)
	s.events = append([]Event(nil), s.events[cutoff:]...)
	return due
}

func before(left, right Event) bool {
	if left.Due != right.Due {
		return left.Due < right.Due
	}
	if left.Priority != right.Priority {
		return left.Priority < right.Priority
	}
	if left.TargetID != right.TargetID {
		return left.TargetID < right.TargetID
	}
	return left.Sequence < right.Sequence
}

// Manual is an injected logical clock for tests and deterministic engines.
type Manual struct{ now LogicalTime }

func NewManual(start LogicalTime) *Manual { return &Manual{now: start} }
func (m *Manual) Now() LogicalTime        { return m.now }
func (m *Manual) AdvanceTo(next LogicalTime) error {
	if next < m.now {
		return ErrTimeReversal
	}
	m.now = next
	return nil
}
func (m *Manual) AdvanceBy(delta LogicalTime) error {
	if delta < 0 {
		return ErrTimeReversal
	}
	return m.AdvanceTo(m.now + delta)
}
