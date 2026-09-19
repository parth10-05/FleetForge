package postgres

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/parth10-05/fleetforge/internal/domain/reason"
)

func TestValidateReplayRecordsRejectsCorruptionAndSequenceGaps(t *testing.T) {
	valid := ReplayRecord{ID: "one", AggregateType: "TARGET_UPDATE", AggregateID: "aggregate", AggregateRevision: 1, DecisionSequence: 0, LogicalTime: time.Now(), ReducerVersion: "phase-02", PolicyVersion: "policy-v1", NormalizedFacts: json.RawMessage(`{"eligible":true}`), Result: "SUBMITTING", Reason: reason.CodeTargetSubmissionNotAuthorized}
	if err := ValidateReplayRecords([]ReplayRecord{valid}); err != nil {
		t.Fatalf("valid replay=%v", err)
	}
	invalid := valid
	invalid.DecisionSequence = 2
	if err := ValidateReplayRecords([]ReplayRecord{valid, invalid}); !errors.Is(err, ErrCorruptPersistedValue) {
		t.Fatalf("gap=%v, want corruption", err)
	}
	invalid = valid
	invalid.NormalizedFacts = json.RawMessage(`[]`)
	if err := ValidateReplayRecords([]ReplayRecord{invalid}); !errors.Is(err, ErrCorruptPersistedValue) {
		t.Fatalf("facts=%v, want corruption", err)
	}
}
