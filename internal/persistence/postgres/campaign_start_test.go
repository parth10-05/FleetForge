package postgres

import (
	"errors"
	"testing"
	"time"
)

func TestStartDecisionRequiresReadyRevisionEpochAndAuthorization(t *testing.T) {
	request := validStartCampaignRequest()
	decision, err := startDecision("READY", 7, 3, request)
	if err != nil {
		t.Fatalf("startDecision: %v", err)
	}
	if got, want := decision.Aggregate.State.String(), "RUNNING_CANARY"; got != want {
		t.Fatalf("next state = %q, want %q", got, want)
	}
	if got, want := decision.Aggregate.Revision.Uint64(), uint64(8); got != want {
		t.Fatalf("next revision = %d, want %d", got, want)
	}
	if got, want := decision.Aggregate.CommandEpoch.Uint64(), uint64(3); got != want {
		t.Fatalf("next epoch = %d, want %d", got, want)
	}

	request.ExpectedRevision = 6
	if _, err := startDecision("READY", 7, 3, request); !errors.Is(err, ErrCampaignStartConflict) {
		t.Fatalf("revision conflict error = %v, want ErrCampaignStartConflict", err)
	}
	request = validStartCampaignRequest()
	if _, err := startDecision("VALIDATING", 7, 3, request); !errors.Is(err, ErrCampaignStartConflict) {
		t.Fatalf("non-ready state error = %v, want ErrCampaignStartConflict", err)
	}
}

func TestValidateCanaryIntentsRejectsLaterWaveAndDuplicate(t *testing.T) {
	canaries := map[string]lockedCanaryUpdate{
		"00000000-0000-0000-0000-000000000203": {},
	}
	valid := CanaryWorkIntent{DueWorkID: "00000000-0000-0000-0000-000000000201", CommandID: "00000000-0000-0000-0000-000000000202", TargetUpdateID: "00000000-0000-0000-0000-000000000203"}
	if err := validateCanaryIntents([]CanaryWorkIntent{valid}, canaries); err != nil {
		t.Fatalf("valid canary intent: %v", err)
	}
	valid.TargetUpdateID = "00000000-0000-0000-0000-000000000204"
	if err := validateCanaryIntents([]CanaryWorkIntent{valid}, canaries); !errors.Is(err, ErrCampaignStartConflict) {
		t.Fatalf("later-wave intent error = %v, want ErrCampaignStartConflict", err)
	}
}

func validStartCampaignRequest() StartCampaignRequest {
	return StartCampaignRequest{
		CampaignID: "00000000-0000-0000-0000-000000000101", ExpectedRevision: 7, ExpectedEpoch: 3,
		CommandID: "00000000-0000-0000-0000-000000000102", ActorID: "system",
		CorrelationID: "00000000-0000-0000-0000-000000000103", CausationID: "00000000-0000-0000-0000-000000000104",
		Now: time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC), AuditAction: "START_CANARY", AuditEventSequence: 0,
		OutboxEventID: "00000000-0000-0000-0000-000000000105", OutboxSchemaVersion: 1,
		DecisionInputID: "00000000-0000-0000-0000-000000000106", DecisionSequence: 0, ReducerVersion: "campaign-reducer/v1",
	}
}
