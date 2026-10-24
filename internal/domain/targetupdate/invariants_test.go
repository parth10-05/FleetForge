package targetupdate

import "testing"

func TestValidateAggregateEnforcesResumeStepOwnership(t *testing.T) {
	for _, aggregate := range []Aggregate{{State: StatePending}, {State: StateRetryWait, ResumeStep: ResumeStepPrecheck}, {State: StateRetryWait, ResumeStep: ResumeStepSubmitAdmission}, {State: StateRetryWait, ResumeStep: ResumeStepTaskObserve}, {State: StateRetryWait, ResumeStep: ResumeStepRebootWait}, {State: StateRetryWait, ResumeStep: ResumeStepVerifyVersion}, {State: StateRetryWait, ResumeStep: ResumeStepVerifyHealth}, {State: StateRetryWait, ResumeStep: ResumeStepAmbiguityReconcile}} {
		if !ValidateAggregate(aggregate) {
			t.Fatalf("valid aggregate rejected: %#v", aggregate)
		}
	}
	for _, aggregate := range []Aggregate{{State: StateRetryWait}, {State: StateRetryWait, ResumeStep: "UNKNOWN"}, {State: StateVerifying, ResumeStep: ResumeStepVerifyVersion}, {State: "UNKNOWN"}} {
		if ValidateAggregate(aggregate) {
			t.Fatalf("invalid aggregate accepted: %#v", aggregate)
		}
	}
}
