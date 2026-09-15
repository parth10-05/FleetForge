package postgres

import (
	"errors"
	"testing"

	"github.com/parth10-05/fleetforge/internal/domain/campaign"
	"github.com/parth10-05/fleetforge/internal/domain/reason"
	"github.com/parth10-05/fleetforge/internal/domain/targetupdate"
)

// ─── CampaignRowState ────────────────────────────────────────────────────────

func TestCampaignRowStateAcceptsAllKnownStates(t *testing.T) {
	for _, state := range campaign.AllStates() {
		got, err := CampaignRowState(string(state))
		if err != nil {
			t.Errorf("CampaignRowState(%q) error = %v, want nil", state, err)
		}
		if got != state {
			t.Errorf("CampaignRowState(%q) = %q, want %q", state, got, state)
		}
	}
}

func TestCampaignRowStateRejectsUnknownValues(t *testing.T) {
	for _, raw := range []string{"", "UNKNOWN", "draft", "ACTIVE", "CORRUPT_STATE"} {
		_, err := CampaignRowState(raw)
		if err == nil {
			t.Errorf("CampaignRowState(%q) = nil error, want ErrCorruptPersistedValue", raw)
			continue
		}
		if !isCorrupt(err) {
			t.Errorf("CampaignRowState(%q) error = %v, want ErrCorruptPersistedValue", raw, err)
		}
	}
}

// ─── CampaignRowSuspendedPhase ───────────────────────────────────────────────

func TestCampaignRowSuspendedPhaseAcceptsNil(t *testing.T) {
	got, err := CampaignRowSuspendedPhase(nil)
	if err != nil || got != "" {
		t.Errorf("CampaignRowSuspendedPhase(nil) = %q, %v; want empty, nil", got, err)
	}
}

func TestCampaignRowSuspendedPhaseAcceptsSuspendableStates(t *testing.T) {
	for _, s := range []campaign.State{campaign.StateRunningCanary, campaign.StateEvaluatingCanary, campaign.StateRunningWaves} {
		raw := string(s)
		got, err := CampaignRowSuspendedPhase(&raw)
		if err != nil {
			t.Errorf("CampaignRowSuspendedPhase(%q) error = %v, want nil", s, err)
		}
		if got != s {
			t.Errorf("CampaignRowSuspendedPhase(%q) = %q, want %q", s, got, s)
		}
	}
}

func TestCampaignRowSuspendedPhaseRejectsNonSuspendableStates(t *testing.T) {
	for _, s := range []campaign.State{campaign.StateDraft, campaign.StateReady, campaign.StatePaused, campaign.StateCompleted} {
		raw := string(s)
		_, err := CampaignRowSuspendedPhase(&raw)
		if err == nil {
			t.Errorf("CampaignRowSuspendedPhase(%q) = nil error, want ErrCorruptPersistedValue", s)
		}
	}
}

func TestCampaignRowSuspendedPhaseRejectsUnknownValues(t *testing.T) {
	raw := "BOGUS_PHASE"
	_, err := CampaignRowSuspendedPhase(&raw)
	if !isCorrupt(err) {
		t.Errorf("CampaignRowSuspendedPhase(%q) error = %v, want ErrCorruptPersistedValue", raw, err)
	}
}

// ─── TargetUpdateRowState ────────────────────────────────────────────────────

func TestTargetUpdateRowStateAcceptsAllKnownStates(t *testing.T) {
	for _, state := range targetupdate.AllStates() {
		got, err := TargetUpdateRowState(string(state))
		if err != nil {
			t.Errorf("TargetUpdateRowState(%q) error = %v, want nil", state, err)
		}
		if got != state {
			t.Errorf("TargetUpdateRowState(%q) = %q, want %q", state, got, state)
		}
	}
}

func TestTargetUpdateRowStateRejectsUnknownValues(t *testing.T) {
	for _, raw := range []string{"", "UNKNOWN", "pending", "CORRUPT"} {
		_, err := TargetUpdateRowState(raw)
		if !isCorrupt(err) {
			t.Errorf("TargetUpdateRowState(%q) error = %v, want ErrCorruptPersistedValue", raw, err)
		}
	}
}

// ─── TargetUpdateRowResumeStep ───────────────────────────────────────────────

func TestTargetUpdateRowResumeStepAcceptsNil(t *testing.T) {
	got, err := TargetUpdateRowResumeStep(nil)
	if err != nil || got != "" {
		t.Errorf("TargetUpdateRowResumeStep(nil) = %q, %v; want empty, nil", got, err)
	}
}

func TestTargetUpdateRowResumeStepAcceptsKnownSteps(t *testing.T) {
	for _, step := range []targetupdate.ResumeStep{
		targetupdate.ResumeStepPrecheck,
		targetupdate.ResumeStepSubmitting,
		targetupdate.ResumeStepWaitingReboot,
		targetupdate.ResumeStepVerifying,
	} {
		raw := string(step)
		got, err := TargetUpdateRowResumeStep(&raw)
		if err != nil {
			t.Errorf("TargetUpdateRowResumeStep(%q) error = %v, want nil", step, err)
		}
		if got != step {
			t.Errorf("TargetUpdateRowResumeStep(%q) = %q, want %q", step, got, step)
		}
	}
}

func TestTargetUpdateRowResumeStepRejectsUnknownValues(t *testing.T) {
	raw := "BOGUS_STEP"
	_, err := TargetUpdateRowResumeStep(&raw)
	if !isCorrupt(err) {
		t.Errorf("TargetUpdateRowResumeStep(%q) error = %v, want ErrCorruptPersistedValue", raw, err)
	}
}

// ─── AuditEventRowReasonCode ──────────────────────────────────────────────────

func TestAuditEventRowReasonCodeAcceptsAllKnownCodes(t *testing.T) {
	for _, code := range reason.AllCodes() {
		got, err := AuditEventRowReasonCode(string(code))
		if err != nil {
			t.Errorf("AuditEventRowReasonCode(%q) error = %v, want nil", code, err)
		}
		if got != code {
			t.Errorf("AuditEventRowReasonCode(%q) = %q, want %q", code, got, code)
		}
	}
}

func TestAuditEventRowReasonCodeRejectsUnknownValues(t *testing.T) {
	for _, raw := range []string{"", "UNKNOWN", "corrupt.reason"} {
		_, err := AuditEventRowReasonCode(raw)
		if !isCorrupt(err) {
			t.Errorf("AuditEventRowReasonCode(%q) error = %v, want ErrCorruptPersistedValue", raw, err)
		}
	}
}

// ─── ParseDueWorkSubjectType ──────────────────────────────────────────────────

func TestParseDueWorkSubjectTypeAcceptsKnownTypes(t *testing.T) {
	for _, subjectType := range []DueWorkSubjectType{
		DueWorkSubjectTargetUpdate,
		DueWorkSubjectCampaign,
		DueWorkSubjectArtifact,
		DueWorkSubjectValidation,
	} {
		got, err := ParseDueWorkSubjectType(string(subjectType))
		if err != nil {
			t.Errorf("ParseDueWorkSubjectType(%q) error = %v, want nil", subjectType, err)
		}
		if got != subjectType {
			t.Errorf("ParseDueWorkSubjectType(%q) = %q, want %q", subjectType, got, subjectType)
		}
	}
}

func TestParseDueWorkSubjectTypeRejectsUnknownValues(t *testing.T) {
	for _, raw := range []string{"", "UNKNOWN", "target_update", "BOGUS"} {
		_, err := ParseDueWorkSubjectType(raw)
		if !isCorrupt(err) {
			t.Errorf("ParseDueWorkSubjectType(%q) error = %v, want ErrCorruptPersistedValue", raw, err)
		}
	}
}

// ─── ParseIdempotencyState ────────────────────────────────────────────────────

func TestParseIdempotencyStateAcceptsKnownStates(t *testing.T) {
	for _, state := range []IdempotencyState{
		IdempotencyStateInProgress,
		IdempotencyStateCompleted,
		IdempotencyStateRecoveryRequired,
	} {
		got, err := ParseIdempotencyState(string(state))
		if err != nil {
			t.Errorf("ParseIdempotencyState(%q) error = %v, want nil", state, err)
		}
		if got != state {
			t.Errorf("ParseIdempotencyState(%q) = %q, want %q", state, got, state)
		}
	}
}

func TestParseIdempotencyStateRejectsUnknownValues(t *testing.T) {
	for _, raw := range []string{"", "DONE", "in_progress", "CORRUPT"} {
		_, err := ParseIdempotencyState(raw)
		if !isCorrupt(err) {
			t.Errorf("ParseIdempotencyState(%q) error = %v, want ErrCorruptPersistedValue", raw, err)
		}
	}
}

// ─── ParseDecisionInputAggregateType ──────────────────────────────────────────

func TestParseDecisionInputAggregateTypeAcceptsKnownTypes(t *testing.T) {
	for _, aggregateType := range []DecisionInputAggregateType{
		DecisionInputAggregateCampaign,
		DecisionInputAggregateTargetUpdate,
	} {
		got, err := ParseDecisionInputAggregateType(string(aggregateType))
		if err != nil {
			t.Errorf("ParseDecisionInputAggregateType(%q) error = %v, want nil", aggregateType, err)
		}
		if got != aggregateType {
			t.Errorf("ParseDecisionInputAggregateType(%q) = %q, want %q", aggregateType, got, aggregateType)
		}
	}
}

func TestParseDecisionInputAggregateTypeRejectsUnknownValues(t *testing.T) {
	for _, raw := range []string{"", "ARTIFACT", "campaign", "CORRUPT"} {
		_, err := ParseDecisionInputAggregateType(raw)
		if !isCorrupt(err) {
			t.Errorf("ParseDecisionInputAggregateType(%q) error = %v, want ErrCorruptPersistedValue", raw, err)
		}
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func isCorrupt(err error) bool {
	return err != nil && errors.Is(err, ErrCorruptPersistedValue)
}
