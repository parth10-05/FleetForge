package core

import "github.com/parth10-05/fleetforge/internal/simulator/clock"

// Command is a closed, immutable set of engine inputs.
type Command interface{ command() }

type AdvanceTo struct{ Time clock.LogicalTime }

func (AdvanceTo) command() {}

// SetHealth changes synthetic observation state only. It does not model an
// update route or an external device request.
type SetHealth struct {
	TargetID string
	Healthy  bool
}

func (SetHealth) command() {}

// ScheduleReachability sets a future synthetic reachability state.
type ScheduleReachability struct {
	TargetID  string
	At        clock.LogicalTime
	Reachable bool
	Priority  int
}

func (ScheduleReachability) command() {}
