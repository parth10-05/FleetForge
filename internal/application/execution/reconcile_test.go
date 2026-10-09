package execution

import (
	"context"
	"testing"
	"time"
)

type reconciliationClaimer struct{ work WorkFence }

func (c reconciliationClaimer) ClaimReconciliationWork(context.Context, string, string) (WorkFence, bool, error) {
	return c.work, true, nil
}

type reconciliationRepository struct {
	snapshot  ReconciliationSnapshot
	committed ReconciliationDecision
}

func (r *reconciliationRepository) LoadReconciliation(context.Context, WorkFence) (ReconciliationSnapshot, error) {
	return r.snapshot, nil
}
func (r *reconciliationRepository) CommitReconciliation(_ context.Context, _ WorkFence, decision ReconciliationDecision) error {
	r.committed = decision
	return nil
}

type acceptanceReader struct{ journal AcceptanceJournal }

func (r acceptanceReader) ReadAcceptance(context.Context, string, string, time.Time, time.Time) (AcceptanceJournal, error) {
	return r.journal, nil
}

type failingAcceptanceReader struct{ calls int }

func (r *failingAcceptanceReader) ReadAcceptance(context.Context, string, string, time.Time, time.Time) (AcceptanceJournal, error) {
	r.calls++
	return AcceptanceJournal{}, nil
}

func TestEvaluateAcceptanceJournalNeverGrantsRetryFromAbsenceAlone(t *testing.T) {
	attempt := AttemptSnapshot{AttemptID: "attempt-1", RequestFingerprint: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	decision, outcome, err := EvaluateAcceptanceJournal(attempt, AcceptanceJournal{RunID: "run", CheckpointHash: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"})
	if err != nil || outcome != reconciliationNeedsAttention || decision.ProvenAbsent || decision.LateCallerExcluded {
		t.Fatalf("decision=%+v outcome=%s err=%v", decision, outcome, err)
	}
}

func TestEvaluateAcceptanceJournalAdoptsMatchingAcceptedAttempt(t *testing.T) {
	attempt := AttemptSnapshot{AttemptID: "attempt-1", RequestFingerprint: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	decision, outcome, err := EvaluateAcceptanceJournal(attempt, AcceptanceJournal{RunID: "run", CheckpointHash: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Entries: []AcceptanceEntry{{AttemptID: "attempt-1", RequestFingerprint: attempt.RequestFingerprint, Received: true, Accepted: true, Sequence: 8}}})
	if err != nil || outcome != reconciliationAdopt || decision.JournalSequenceStart != 8 || decision.JournalSequenceEnd != 8 {
		t.Fatalf("decision=%+v outcome=%s err=%v", decision, outcome, err)
	}
}

func TestReconciliationWorkerAdoptsOnlyIndependentMatchingAcceptance(t *testing.T) {
	attempt := AttemptSnapshot{AttemptID: "attempt-1", TargetID: "target-1", RequestFingerprint: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", StartedAt: time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)}
	work := WorkFence{DueWorkID: "work", CommandID: "command", LeaseToken: "lease"}
	repository := &reconciliationRepository{snapshot: ReconciliationSnapshot{Attempt: attempt}}
	worker := ReconciliationWorker{Claimer: reconciliationClaimer{work: work}, Repository: repository, Journal: acceptanceReader{journal: AcceptanceJournal{RunID: "run", CheckpointHash: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Entries: []AcceptanceEntry{{AttemptID: attempt.AttemptID, RequestFingerprint: attempt.RequestFingerprint, Received: true, Accepted: true, Sequence: 5}}}}, RunID: "run", Now: func() time.Time { return time.Date(2026, 10, 9, 0, 1, 0, 0, time.UTC) }}
	claimed, err := worker.RunOnce(context.Background(), "reconciler", "token")
	if err != nil || !claimed || repository.committed.Outcome != reconciliationAdopt || repository.committed.Work != work {
		t.Fatalf("claimed=%t err=%v decision=%+v", claimed, err, repository.committed)
	}
}

func TestReconciliationWorkerFailsClosedWithoutAttemptTimeBoundary(t *testing.T) {
	work := WorkFence{DueWorkID: "work", CommandID: "command", LeaseToken: "lease"}
	reader := &failingAcceptanceReader{}
	worker := ReconciliationWorker{Claimer: reconciliationClaimer{work: work}, Repository: &reconciliationRepository{snapshot: ReconciliationSnapshot{Attempt: AttemptSnapshot{AttemptID: "attempt-1", TargetID: "target-1"}}}, Journal: reader, RunID: "run", Now: time.Now}
	if _, err := worker.RunOnce(context.Background(), "reconciler", "token"); err == nil || reader.calls != 0 {
		t.Fatalf("err=%v reader calls=%d", err, reader.calls)
	}
}
