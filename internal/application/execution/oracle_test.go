package execution

import "testing"

func oracleDigest(letter string) string {
	return "sha256:" + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
}

func TestVerifyIndependentJournalAcceptsOneMatchingOperation(t *testing.T) {
	attempt := OracleAttempt{AttemptID: "attempt-1", IntendedUpdateID: "update-1", RequestFingerprint: oracleDigest("a")}
	journal := AcceptanceJournal{RunID: "run-1", CheckpointHash: oracleDigest("b"), Entries: []AcceptanceEntry{{AttemptID: attempt.AttemptID, RequestFingerprint: attempt.RequestFingerprint, Received: true, Accepted: true, Sequence: 1}}}
	if err := VerifyIndependentJournal([]OracleAttempt{attempt}, journal); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyIndependentJournalRejectsDuplicateAcceptedOperation(t *testing.T) {
	attempts := []OracleAttempt{{AttemptID: "attempt-1", IntendedUpdateID: "update-1", RequestFingerprint: oracleDigest("a")}, {AttemptID: "attempt-2", IntendedUpdateID: "update-1", RequestFingerprint: oracleDigest("b")}}
	journal := AcceptanceJournal{RunID: "run-1", CheckpointHash: oracleDigest("c"), Entries: []AcceptanceEntry{{AttemptID: "attempt-1", RequestFingerprint: attempts[0].RequestFingerprint, Accepted: true, Sequence: 1}, {AttemptID: "attempt-2", RequestFingerprint: attempts[1].RequestFingerprint, Accepted: true, Sequence: 2}}}
	if err := VerifyIndependentJournal(attempts, journal); err == nil {
		t.Fatal("duplicate accepted operation was not rejected")
	}
}

func TestVerifyIndependentJournalRejectsFalseProvenAbsent(t *testing.T) {
	attempt := OracleAttempt{AttemptID: "attempt-1", IntendedUpdateID: "update-1", RequestFingerprint: oracleDigest("a"), Resolution: "PROVEN_ABSENT", LateCallerExcluded: true}
	journal := AcceptanceJournal{RunID: "run-1", CheckpointHash: oracleDigest("b"), Entries: []AcceptanceEntry{{AttemptID: attempt.AttemptID, RequestFingerprint: attempt.RequestFingerprint, Received: true, Sequence: 1}}}
	if err := VerifyIndependentJournal([]OracleAttempt{attempt}, journal); err == nil {
		t.Fatal("false proven-absent resolution was not rejected")
	}
}

func TestVerifyIndependentJournalRejectsNonCanonicalCheckpoint(t *testing.T) {
	attempt := OracleAttempt{AttemptID: "attempt-1", IntendedUpdateID: "update-1", RequestFingerprint: oracleDigest("a")}
	journal := AcceptanceJournal{RunID: "run-1", CheckpointHash: "not-a-checkpoint"}
	if err := VerifyIndependentJournal([]OracleAttempt{attempt}, journal); err == nil {
		t.Fatal("noncanonical checkpoint was accepted")
	}
}

func TestVerifyIndependentJournalRejectsAcceptedEntryWithoutReceivedEvidence(t *testing.T) {
	attempt := OracleAttempt{AttemptID: "attempt-1", IntendedUpdateID: "update-1", RequestFingerprint: oracleDigest("a")}
	journal := AcceptanceJournal{RunID: "run-1", CheckpointHash: oracleDigest("b"), Entries: []AcceptanceEntry{{AttemptID: attempt.AttemptID, RequestFingerprint: attempt.RequestFingerprint, Accepted: true, Sequence: 1}}}
	if err := VerifyIndependentJournal([]OracleAttempt{attempt}, journal); err == nil {
		t.Fatal("accepted entry without a retained receive record was accepted")
	}
}

func TestVerifyIndependentJournalRejectsDurableAcceptanceMissingFromJournal(t *testing.T) {
	attempt := OracleAttempt{AttemptID: "attempt-1", IntendedUpdateID: "update-1", RequestFingerprint: oracleDigest("a"), Resolution: "ACCEPTED"}
	journal := AcceptanceJournal{RunID: "run-1", CheckpointHash: oracleDigest("b")}
	if err := VerifyIndependentJournal([]OracleAttempt{attempt}, journal); err == nil {
		t.Fatal("durable accepted attempt without retained journal evidence was accepted")
	}
}
