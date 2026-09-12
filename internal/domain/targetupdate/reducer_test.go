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
