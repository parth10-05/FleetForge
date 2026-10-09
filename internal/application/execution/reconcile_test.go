package execution

import "testing"

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
