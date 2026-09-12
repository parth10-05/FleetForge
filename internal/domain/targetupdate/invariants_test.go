package targetupdate

import "testing"

func TestValidateAggregateEnforcesResumeStepOwnership(t *testing.T) {
	for _, aggregate := range []Aggregate{{State: StatePending}, {State: StateRetryWait, ResumeStep: ResumeStepPrecheck}, {State: StateRetryWait, ResumeStep: ResumeStepSubmitting}, {State: StateRetryWait, ResumeStep: ResumeStepWaitingReboot}, {State: StateRetryWait, ResumeStep: ResumeStepVerifying}} {
		if !ValidateAggregate(aggregate) {
			t.Fatalf("valid aggregate rejected: %#v", aggregate)
		}
	}
	for _, aggregate := range []Aggregate{{State: StateRetryWait}, {State: StateRetryWait, ResumeStep: "UNKNOWN"}, {State: StateVerifying, ResumeStep: ResumeStepVerifying}, {State: "UNKNOWN"}} {
		if ValidateAggregate(aggregate) {
			t.Fatalf("invalid aggregate accepted: %#v", aggregate)
		}
	}
}
