package campaignplan

import (
	"testing"
	"time"

	"github.com/parth10-05/fleetforge/internal/domain/reason"
)

func TestEvaluateCanaryUsesInclusiveFailureBudgetAndFreshScheduledHealth(t *testing.T) {
	input := validEvaluationInput()
	decision, err := EvaluateCanary(input)
	if err != nil || decision.State != EvaluationPassed || decision.Reason != reason.CodeCampaignCanaryPassed {
		t.Fatalf("passing decision = %#v, err=%v", decision, err)
	}
	input.Outcomes[1].UpdateState = "FAILED_TERMINAL"
	decision, err = EvaluateCanary(input)
	if err != nil || decision.State != EvaluationFailed || decision.Reason != reason.CodeCampaignBudgetThresholdReached {
		t.Fatalf("inclusive threshold decision = %#v, err=%v", decision, err)
	}
	input = validEvaluationInput()
	input.Health[0].ScheduledAt = input.LogicalCut.Add(time.Second)
	decision, err = EvaluateCanary(input)
	if err != nil || decision.State != EvaluationPending || decision.Reason != reason.CodeCampaignCanaryHealthBlocked {
		t.Fatalf("unscheduled health decision = %#v, err=%v", decision, err)
	}
}

func TestEvaluateCanaryFailsClosedForAttentionAndDuration(t *testing.T) {
	input := validEvaluationInput()
	input.Outcomes[0].UpdateState = "NEEDS_ATTENTION"
	decision, err := EvaluateCanary(input)
	if err != nil || decision.State != EvaluationPaused || decision.Reason != reason.CodeCampaignCanaryNeedsAttention {
		t.Fatalf("attention decision = %#v, err=%v", decision, err)
	}
	input = validEvaluationInput()
	input.LogicalCut = input.StartedAt.Add(time.Duration(input.Policy.MaximumDurationSeconds+1) * time.Second)
	decision, err = EvaluateCanary(input)
	if err != nil || decision.State != EvaluationPaused || decision.Reason != reason.CodeCampaignCanaryDurationExpired {
		t.Fatalf("duration decision = %#v, err=%v", decision, err)
	}
}

func TestEvaluateCanaryPausesForExhaustedFailureBudgetBeforeHealthGate(t *testing.T) {
	input := validEvaluationInput()
	input.Outcomes[0].UpdateState = "FAILED_TERMINAL"
	input.Health = nil
	decision, err := EvaluateCanary(input)
	if err != nil || decision.State != EvaluationFailed || decision.Reason != reason.CodeCampaignBudgetThresholdReached {
		t.Fatalf("failed canary decision = %#v, err=%v", decision, err)
	}
}

func TestEvaluateCanaryBlocksOutsideFrozenMaintenanceWindow(t *testing.T) {
	input := validEvaluationInput()
	input.Policy.Maintenance = MaintenanceWindow{Timezone: "UTC", Weekdays: []int{6}, StartMinute: 120, EndMinute: 180}
	decision, err := EvaluateCanary(input)
	if err != nil || decision.State != EvaluationPending || decision.Reason != reason.CodeCampaignWaveBlocked {
		t.Fatalf("maintenance decision = %#v, err=%v", decision, err)
	}
}

func validEvaluationInput() EvaluationInput {
	started := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	policy := validPolicy(Size{Unit: SizeUnitAbsolute, Value: 1})
	policy.FailureBudget = Fraction{Numerator: 1, Denominator: 2}
	policy.Maintenance = MaintenanceWindow{Timezone: "UTC", Weekdays: []int{6}, StartMinute: 0, EndMinute: 120}
	return EvaluationInput{Policy: policy, CandidateCount: 2, StartedAt: started, LogicalCut: started.Add(time.Minute), Outcomes: []TargetOutcome{{TargetID: "a", UpdateState: "SUCCEEDED", ActiveEnrollment: true}, {TargetID: "b", UpdateState: "SUCCEEDED", ActiveEnrollment: true}}, Health: []HealthSample{{TargetID: "a", Check: "boot", ScheduledAt: started, ObservedAt: started, Passed: true}, {TargetID: "b", Check: "boot", ScheduledAt: started, ObservedAt: started, Passed: true}}}
}

func TestEvaluateCanaryUsesFullCampaignCandidateCount(t *testing.T) {
	input := validEvaluationInput()
	input.CandidateCount = 6
	input.Outcomes = input.Outcomes[:1]
	input.Health = input.Health[:1]
	input.Policy.Limits = Limits{Global: 6, Site: 3, Rack: 2}
	decision, err := EvaluateCanary(input)
	if err != nil || decision.State != EvaluationPassed {
		t.Fatalf("full campaign candidate count decision = %#v, err=%v", decision, err)
	}
}
