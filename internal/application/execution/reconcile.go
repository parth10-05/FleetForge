package execution

import (
	"fmt"
	"strings"
)

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
		decision.ReasonCode = "execution.journal.accepted"
		return decision, reconciliationAdopt, nil
	}
	if foundRequest {
		decision.ReasonCode = "execution.journal.request_without_acceptance"
		return decision, reconciliationNeedsAttention, nil
	}
	// Do not infer absence here. A late worker holding a preexisting delivery
	// may append after this checkpoint; an independent exclusion mechanism is
	// required before ProvenAbsent/LateCallerExcluded can ever be true.
	return decision, reconciliationNeedsAttention, nil
}
