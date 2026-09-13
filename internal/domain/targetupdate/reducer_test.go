package targetupdate

import (
	"github.com/parth10-05/fleetforge/internal/domain/policy"
	"github.com/parth10-05/fleetforge/internal/domain/value"
	"testing"
)

func targetID(t *testing.T) value.CommandID {
	t.Helper()
	id, e := value.ParseCommandID("018f2a60-0b71-7dd3-8ae8-2d4f2d2ae9c5")
	if e != nil {
		t.Fatal(e)
	}
	return id
}

func TestReduceRequiresSpecificSafetyProofs(t *testing.T) {
	id := targetID(t)
	t.Run("submission needs persisted exact admission", func(t *testing.T) {
		before := Aggregate{State: StateSubmitting, CommandEpoch: 2}
		input := authorizedTargetInput(id, CommandAdmitSubmission, 0, 2)
		input.Submission = policy.NewPersistedSubmissionAdmission(id, 1)
		decision := Reduce(before, input)
		if decision.Class != DecisionUnsafeCondition || decision.SubmitIntent || decision.Aggregate != before {
			t.Fatalf("unfenced submission = %#v", decision)
		}
	})
	t.Run("retry needs independent absence and late-caller exclusion", func(t *testing.T) {
		before := Aggregate{State: StateNeedsAttention}
		input := authorizedTargetInput(id, CommandAttentionRetry, 0, 0)
		input.RetryAbsence = policy.IndependentAbsenceProof{}
		decision := Reduce(before, input)
		if decision.Class != DecisionUnsafeCondition || decision.Aggregate != before {
			t.Fatalf("unproven retry = %#v", decision)
		}
	})
	t.Run("success needs version and health evidence", func(t *testing.T) {
		before := Aggregate{State: StateVerifying}
		input := authorizedTargetInput(id, CommandVerificationSucceeded, 0, 0)
		input.SuccessEvidence = policy.VersionAndHealthSuccess{}
		decision := Reduce(before, input)
		if decision.Class != DecisionUnsafeCondition || decision.Aggregate != before {
			t.Fatalf("unverified success = %#v", decision)
		}
	})
}
func TestReducePreservesRetryAndBlocksStaleEvidence(t *testing.T) {
	a := Aggregate{State: StateVerifying, Revision: 2}
	d := Reduce(a, authorizedTargetInput(targetID(t), CommandVerificationRetryable, 2, 0))
	if d.Class != DecisionAllowed || d.Aggregate.State != StateRetryWait || d.Aggregate.ResumeStep != ResumeStepVerifying {
		t.Fatalf("retry=%+v", d)
	}
	stale := authorizedTargetInput(targetID(t), CommandAdmitSubmission, 3, 0)
	stale.EvidenceOlder = true
	staleDecision := Reduce(d.Aggregate, stale)
	if staleDecision.Class != DecisionNoOp || staleDecision.SubmitIntent {
		t.Fatalf("stale=%+v", staleDecision)
	}
}
func TestReduceAttentionAndSubmissionEpoch(t *testing.T) {
	a := Aggregate{State: StateNeedsAttention, Revision: 1, CommandEpoch: 4}
	d := Reduce(a, authorizedTargetInput(targetID(t), CommandAttentionRetry, 1, 4))
	if d.Class != DecisionAllowed || d.Aggregate.ResumeStep != ResumeStepSubmitting || d.Attention != AttentionExitProofRetry {
		t.Fatalf("attention=%+v", d)
	}
	b := Aggregate{State: StateSubmitting, CommandEpoch: 4}
	conflict := Reduce(b, authorizedTargetInput(targetID(t), CommandAdmitSubmission, 0, 3))
	if conflict.Class != DecisionConflict || conflict.SubmitIntent {
		t.Fatalf("conflict=%+v", conflict)
	}
}

func TestReduceRetryWaitRequiresMatchingPersistedResumeStep(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		resume  ResumeStep
		command Command
	}{
		{name: "precheck cannot admit submission", resume: ResumeStepPrecheck, command: CommandAdmitSubmission},
		{name: "precheck cannot resume reboot observation", resume: ResumeStepPrecheck, command: CommandRebootObserved},
		{name: "precheck cannot resume verification", resume: ResumeStepPrecheck, command: CommandVerificationRetryable},
		{name: "submitting cannot restart precheck", resume: ResumeStepSubmitting, command: CommandRetryDue},
		{name: "submitting cannot resume reboot observation", resume: ResumeStepSubmitting, command: CommandRebootObserved},
		{name: "submitting cannot resume verification", resume: ResumeStepSubmitting, command: CommandVerificationRetryable},
		{name: "waiting reboot cannot restart precheck", resume: ResumeStepWaitingReboot, command: CommandRetryDue},
		{name: "waiting reboot cannot admit submission", resume: ResumeStepWaitingReboot, command: CommandAdmitSubmission},
		{name: "waiting reboot cannot resume verification", resume: ResumeStepWaitingReboot, command: CommandVerificationRetryable},
		{name: "verifying cannot restart precheck", resume: ResumeStepVerifying, command: CommandRetryDue},
		{name: "verifying cannot admit submission", resume: ResumeStepVerifying, command: CommandAdmitSubmission},
		{name: "verifying cannot resume reboot observation", resume: ResumeStepVerifying, command: CommandRebootObserved},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			before := Aggregate{State: StateRetryWait, ResumeStep: testCase.resume}
			decision := Reduce(before, authorizedTargetInput(targetID(t), testCase.command, 0, 0))
			if decision.Class == DecisionAllowed || decision.Aggregate != before || decision.SubmitIntent {
				t.Fatalf("mismatched retry exit = %#v", decision)
			}
		})
	}
}
