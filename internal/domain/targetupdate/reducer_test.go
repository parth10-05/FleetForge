package targetupdate

import (
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
func TestReducePreservesRetryAndBlocksStaleEvidence(t *testing.T) {
	a := Aggregate{State: StateVerifying, Revision: 2}
	d := Reduce(a, CommandInput{ID: targetID(t), Command: CommandVerificationRetryable, ExpectedRevision: 2, GuardsSatisfied: true})
	if d.Class != DecisionAllowed || d.Aggregate.State != StateRetryWait || d.Aggregate.ResumeStep != ResumeStepVerifying {
		t.Fatalf("retry=%+v", d)
	}
	stale := Reduce(d.Aggregate, CommandInput{ID: targetID(t), Command: CommandAdmitSubmission, ExpectedRevision: 3, EvidenceOlder: true, GuardsSatisfied: true})
	if stale.Class != DecisionNoOp || stale.SubmitIntent {
		t.Fatalf("stale=%+v", stale)
	}
}
func TestReduceAttentionAndSubmissionEpoch(t *testing.T) {
	a := Aggregate{State: StateNeedsAttention, Revision: 1, CommandEpoch: 4}
	d := Reduce(a, CommandInput{ID: targetID(t), Command: CommandAttentionRetry, ExpectedRevision: 1, GuardsSatisfied: true})
	if d.Class != DecisionAllowed || d.Aggregate.ResumeStep != ResumeStepSubmitting || d.Attention != AttentionExitProofRetry {
		t.Fatalf("attention=%+v", d)
	}
	b := Aggregate{State: StateSubmitting, CommandEpoch: 4}
	conflict := Reduce(b, CommandInput{ID: targetID(t), Command: CommandAdmitSubmission, ExpectedEpoch: 3, GuardsSatisfied: true})
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
			decision := Reduce(before, CommandInput{ID: targetID(t), Command: testCase.command, GuardsSatisfied: true})
			if decision.Class == DecisionAllowed || decision.Aggregate != before || decision.SubmitIntent {
				t.Fatalf("mismatched retry exit = %#v", decision)
			}
		})
	}
}
