package campaign

import (
	"testing"

	"github.com/parth10-05/fleetforge/internal/domain/value"
)

func TestAbortFromPausedClearsCapturedPhase(t *testing.T) {
	id, err := value.ParseCommandID("00000000-0000-4000-8000-0000000000d1")
	if err != nil {
		t.Fatal(err)
	}
	decision := Reduce(Aggregate{State: StatePaused, Revision: 3, CommandEpoch: 2, SuspendedPhase: StateRunningCanary}, CommandInput{ID: id, Command: CommandAbort, ExpectedRevision: 3, ExpectedEpoch: 2, GuardsSatisfied: true})
	if decision.Class != DecisionAllowed || decision.Aggregate.State != StateAborting || decision.Aggregate.SuspendedPhase != "" || !ValidateAggregate(decision.Aggregate) {
		t.Fatalf("abort result = %#v", decision)
	}
}
