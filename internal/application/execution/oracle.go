package execution

import (
	"context"
	"fmt"
	"regexp"
)

var oracleSHA256 = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// OracleAttempt is the minimum durable FleetForge record required to compare
// a firmware attempt against the independently retained simulator journal.
// IntendedUpdateID is the stable target-update identity; it deliberately does
// not use a transport message, receipt, or task identifier.
type OracleAttempt struct {
	AttemptID, IntendedUpdateID, RequestFingerprint, Resolution string
	LateCallerExcluded                                          bool
}

// OracleAttemptSource exposes immutable FleetForge execution evidence to an
// independent checker. It intentionally has no mutation, admission, or target
// capability: checking a simulator journal cannot affect an in-flight update.
type OracleAttemptSource interface {
	LoadOracleAttempts(context.Context) ([]OracleAttempt, error)
}

// VerifyPersistedJournal joins retained simulator evidence to the durable
// attempt/admission record before evaluating the safety invariants.
func VerifyPersistedJournal(ctx context.Context, source OracleAttemptSource, journal AcceptanceJournal) error {
	if source == nil {
		return fmt.Errorf("oracle attempt source is required")
	}
	attempts, err := source.LoadOracleAttempts(ctx)
	if err != nil {
		return fmt.Errorf("load durable oracle attempts: %w", err)
	}
	return VerifyIndependentJournal(attempts, journal)
}

// VerifyIndependentJournal rejects safety claims that conflict with the
// simulator's received/accepted journal. It is intentionally strict: an
// unmatched accepted operation, duplicate acceptance for the same intended
// update, or a PROVEN_ABSENT record with any matching receive/accept evidence
// invalidates the evidence set.
func VerifyIndependentJournal(attempts []OracleAttempt, journal AcceptanceJournal) error {
	if journal.RunID == "" || !oracleSHA256.MatchString(journal.CheckpointHash) {
		return fmt.Errorf("oracle requires identified journal evidence")
	}
	byAttempt := make(map[string]OracleAttempt, len(attempts))
	acceptedByUpdate := make(map[string]int)
	matchedReceived := make(map[string]bool)
	matchedAccepted := make(map[string]bool)
	for _, attempt := range attempts {
		if attempt.AttemptID == "" || attempt.IntendedUpdateID == "" || attempt.RequestFingerprint == "" {
			return fmt.Errorf("oracle attempt has incomplete immutable identity")
		}
		if _, exists := byAttempt[attempt.AttemptID]; exists {
			return fmt.Errorf("oracle attempt identity is duplicated")
		}
		byAttempt[attempt.AttemptID] = attempt
	}
	for _, entry := range journal.Entries {
		if entry.Sequence < 1 || (!entry.Received && !entry.Accepted) {
			return fmt.Errorf("oracle journal entry is invalid")
		}
		attempt, found := byAttempt[entry.AttemptID]
		if !found || entry.RequestFingerprint != attempt.RequestFingerprint {
			return fmt.Errorf("oracle journal entry does not match a durable attempt")
		}
		matchedReceived[attempt.AttemptID] = matchedReceived[attempt.AttemptID] || entry.Received
		if entry.Accepted {
			matchedAccepted[attempt.AttemptID] = true
			acceptedByUpdate[attempt.IntendedUpdateID]++
			if acceptedByUpdate[attempt.IntendedUpdateID] > 1 {
				return fmt.Errorf("oracle detected duplicate accepted operation for intended update")
			}
		}
	}
	for attemptID, accepted := range matchedAccepted {
		if accepted && !matchedReceived[attemptID] {
			return fmt.Errorf("oracle accepted operation is missing retained receive evidence")
		}
	}
	for _, attempt := range attempts {
		if attempt.Resolution == "ACCEPTED" && !matchedAccepted[attempt.AttemptID] {
			return fmt.Errorf("oracle missing retained acceptance evidence for durable accepted attempt")
		}
		if attempt.Resolution == "PROVEN_ABSENT" {
			if !attempt.LateCallerExcluded || matchedReceived[attempt.AttemptID] || matchedAccepted[attempt.AttemptID] {
				return fmt.Errorf("oracle rejected false proven-absent resolution")
			}
		}
	}
	return nil
}
