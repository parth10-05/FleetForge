package campaign

import "testing"

func TestValidateAggregateEnforcesSuspendedPhaseOwnership(t *testing.T) {
	for _, aggregate := range []Aggregate{
		{State: StateDraft},
		{State: StatePaused, SuspendedPhase: StateRunningCanary},
		{State: StatePaused, SuspendedPhase: StateEvaluatingCanary},
		{State: StatePaused, SuspendedPhase: StateRunningWaves},
	} {
		if !ValidateAggregate(aggregate) {
			t.Fatalf("valid aggregate rejected: %#v", aggregate)
		}
	}
	for _, aggregate := range []Aggregate{
		{State: StatePaused}, {State: StatePaused, SuspendedPhase: StateReady}, {State: StateReady, SuspendedPhase: StateRunningCanary}, {State: "UNKNOWN"},
	} {
		if ValidateAggregate(aggregate) {
			t.Fatalf("invalid aggregate accepted: %#v", aggregate)
		}
	}
}
