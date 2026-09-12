package campaign

import (
	"github.com/parth10-05/fleetforge/internal/domain/value"
	"testing"
)

const reducerCommandID = "018f2a60-0b71-7dd3-8ae8-2d4f2d2ae9c5"

func mustID(t *testing.T) value.CommandID {
	t.Helper()
	id, err := value.ParseCommandID(reducerCommandID)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
func TestReducePauseResumeAndEpoch(t *testing.T) {
	initial := Aggregate{State: StateRunningCanary, Revision: 3, CommandEpoch: 7}
	pause := Reduce(initial, CommandInput{ID: mustID(t), Command: CommandPause, ExpectedRevision: 3, ExpectedEpoch: 7, GuardsSatisfied: true})
	if pause.Class != DecisionAllowed || pause.Aggregate.State != StatePaused || pause.Aggregate.SuspendedPhase != StateRunningCanary || pause.Aggregate.Revision != 4 || pause.Aggregate.CommandEpoch != 8 {
		t.Fatalf("pause=%+v", pause)
	}
	resume := Reduce(pause.Aggregate, CommandInput{ID: mustID(t), Command: CommandResume, ExpectedRevision: 4, ExpectedEpoch: 8, GuardsSatisfied: true})
	if resume.Class != DecisionAllowed || resume.Aggregate.State != StateRunningCanary || resume.Aggregate.SuspendedPhase != "" || resume.Aggregate.Revision != 5 {
		t.Fatalf("resume=%+v", resume)
	}
}
func TestReduceRejectsStaleDuplicateAndTerminalMutation(t *testing.T) {
	aggregate := Aggregate{State: StateReady, Revision: 2, CommandEpoch: 1}
	stale := Reduce(aggregate, CommandInput{ID: mustID(t), Command: CommandStartCanary, ExpectedRevision: 1, ExpectedEpoch: 1, GuardsSatisfied: true})
	if stale.Class != DecisionConflict || stale.Aggregate != aggregate {
		t.Fatalf("stale=%+v", stale)
	}
	duplicate := Reduce(aggregate, CommandInput{ID: mustID(t), Command: CommandStartCanary, ExpectedRevision: 2, ExpectedEpoch: 1, Duplicate: true, GuardsSatisfied: true})
	if duplicate.Class != DecisionNoOp || len(duplicate.Events) != 0 {
		t.Fatalf("duplicate=%+v", duplicate)
	}
	terminal := Aggregate{State: StateCompleted}
	decision := Reduce(terminal, CommandInput{ID: mustID(t), Command: CommandAbort, GuardsSatisfied: true})
	if decision.Class != DecisionUnsafeCondition || decision.Aggregate != terminal {
		t.Fatalf("terminal=%+v", decision)
	}
}
