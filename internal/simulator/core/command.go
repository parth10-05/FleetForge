package core

import "github.com/parth10-05/fleetforge/internal/simulator/clock"

// Command is the closed, immutable input boundary for the in-memory engine.
// Transport, callback, and persistence packages submit values; none receives
// engine-owned mutable state.
type Command interface{ command() }

type AdvanceTo struct{ Time clock.LogicalTime }

func (AdvanceTo) command() {}

type Observe struct{ TargetID string }

func (Observe) command() {}

// AcceptUpdate creates a distinct synthetic operation and task. AttemptID is a
// correlation input, never a deduplication key.
type AcceptUpdate struct{ TargetID, Version, AttemptID string }

func (AcceptUpdate) command() {}

type CreateSubscription struct{ TargetID, Fingerprint string }

func (CreateSubscription) command() {}

type ConsumeFault struct{ TargetID, FaultID string }

func (ConsumeFault) command() {}

type TransitionKind string

const (
	TransitionTaskRunning TransitionKind = "task-running"
	TransitionRebooting   TransitionKind = "rebooting"
	TransitionInventory   TransitionKind = "inventory"
	TransitionHealth      TransitionKind = "health"
)

type ScheduleTransition struct {
	TargetID, TaskID string
	At               clock.LogicalTime
	Kind             TransitionKind
}

func (ScheduleTransition) command() {}

// Legacy state-only commands remain pure and are retained for compatibility.
type SetHealth struct {
	TargetID string
	Healthy  bool
}

func (SetHealth) command() {}

type ScheduleReachability struct {
	TargetID  string
	At        clock.LogicalTime
	Reachable bool
	Priority  int
}

func (ScheduleReachability) command() {}
