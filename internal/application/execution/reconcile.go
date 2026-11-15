package execution

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ReconciliationRepository intentionally contains no admission or submission
// operation. Reconciliation can adopt independent evidence or retain
// attention, but it can never issue another target-side request.
type ReconciliationRepository interface {
	LoadReconciliation(context.Context, WorkFence) (ReconciliationSnapshot, error)
	CommitReconciliation(context.Context, WorkFence, ReconciliationDecision) error
}

type ReconciliationWorkClaimer interface {
	ClaimReconciliationWork(context.Context, string, string) (WorkFence, bool, error)
}

type ReconciliationWorker struct {
	Claimer    ReconciliationWorkClaimer
	Repository ReconciliationRepository
	Journal    IndependentAcceptanceReader
	RunID      string
	Now        func() time.Time
}

// RunOnce consumes at most one ambiguous-attempt work item. It can only
// reconcile independently retained evidence; retry authorization remains an
// explicit, separately verified administrator decision.
func (worker ReconciliationWorker) RunOnce(ctx context.Context, owner, leaseToken string) (bool, error) {
	if worker.Claimer == nil || worker.Repository == nil || worker.Journal == nil || worker.RunID == "" || worker.Now == nil || owner == "" || leaseToken == "" {
		return false, fmt.Errorf("invalid reconciliation worker")
	}
	work, claimed, err := worker.Claimer.ClaimReconciliationWork(ctx, owner, leaseToken)
	if err != nil || !claimed {
		return claimed, err
	}
	snapshot, err := worker.Repository.LoadReconciliation(ctx, work)
	if err != nil {
		return true, fmt.Errorf("load fenced reconciliation attempt: %w", err)
	}
	if snapshot.Attempt.AttemptID == "" || snapshot.JournalTargetID == "" || snapshot.Attempt.StartedAt.IsZero() {
		return true, fmt.Errorf("reconciliation attempt lacks immutable identity or time boundary")
	}
	until := worker.Now().UTC()
	if until.Before(snapshot.Attempt.StartedAt) {
		return true, fmt.Errorf("reconciliation clock precedes immutable attempt boundary")
	}
	journal, err := worker.Journal.ReadAcceptance(ctx, worker.RunID, snapshot.JournalTargetID, snapshot.Attempt.StartedAt, until)
	if err != nil {
		return true, fmt.Errorf("read independent acceptance journal: %w", err)
	}
	decision, _, err := EvaluateAcceptanceJournal(snapshot.Attempt, journal)
	if err != nil {
		return true, err
	}
	decision.Work = work
	if err := worker.Repository.CommitReconciliation(ctx, work, decision); err != nil {
		return true, fmt.Errorf("commit reconciliation decision: %w", err)
	}
	return true, nil
}

const (
	reconciliationAdopt          = "ADOPT"
	reconciliationNeedsAttention = "NEEDS_ATTENTION"
	reconciliationProvenAbsent   = "PROVEN_ABSENT"
)

// EvaluateAcceptanceJournal is deliberately conservative. A journal that
// lacks this attempt can prove neither that a request was absent nor that the
// original caller cannot still deliver it. Only a separate exclusion proof may
// set LateCallerExcluded and make a retry eligible in a later transaction.
func EvaluateAcceptanceJournal(attempt AttemptSnapshot, journal AcceptanceJournal) (ReconciliationDecision, string, error) {
	if attempt.AttemptID == "" || attempt.RequestFingerprint == "" || journal.RunID == "" || journal.CheckpointHash == "" {
		return ReconciliationDecision{}, "", fmt.Errorf("invalid reconciliation evidence")
	}
	decision := ReconciliationDecision{EvidenceFingerprint: journal.CheckpointHash, JournalRunID: journal.RunID, CheckpointHash: journal.CheckpointHash, ReasonCode: "execution.journal.inconclusive"}
	if !strings.HasPrefix(journal.CheckpointHash, "sha256:") {
		return ReconciliationDecision{}, "", fmt.Errorf("journal checkpoint is not canonical")
	}
	var foundRequest, foundAcceptance bool
	var first, last int64
	for _, entry := range journal.Entries {
		if entry.Sequence < 1 || entry.AttemptID != attempt.AttemptID || entry.RequestFingerprint != attempt.RequestFingerprint {
			continue
		}
		if first == 0 {
			first = entry.Sequence
		}
		last = entry.Sequence
		foundRequest = foundRequest || entry.Received
		foundAcceptance = foundAcceptance || entry.Accepted
	}
	decision.JournalSequenceStart, decision.JournalSequenceEnd = first, last
	if foundAcceptance {
		decision.Outcome = reconciliationAdopt
		decision.ReasonCode = "execution.journal.accepted"
		return decision, reconciliationAdopt, nil
	}
	if foundRequest {
		decision.Outcome = reconciliationNeedsAttention
		decision.ReasonCode = "execution.journal.request_without_acceptance"
		return decision, reconciliationNeedsAttention, nil
	}
	// Do not infer absence here. A late worker holding a preexisting delivery
	// may append after this checkpoint; an independent exclusion mechanism is
	// required before ProvenAbsent/LateCallerExcluded can ever be true.
	decision.Outcome = reconciliationNeedsAttention
	return decision, reconciliationNeedsAttention, nil
}
