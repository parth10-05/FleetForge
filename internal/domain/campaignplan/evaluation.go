package campaignplan

import (
	"errors"
	"time"

	"github.com/parth10-05/fleetforge/internal/domain/policy"
	"github.com/parth10-05/fleetforge/internal/domain/reason"
	"github.com/parth10-05/fleetforge/internal/domain/value"
)

// ErrInvalidEvaluationInput is returned for incomplete or contradictory
// persisted facts. Callers must not turn it into permission to release work.
var ErrInvalidEvaluationInput = errors.New("invalid campaign evaluation input")

type EvaluationState string

const (
	EvaluationPending EvaluationState = "PENDING"
	EvaluationPassed  EvaluationState = "PASSED"
	EvaluationFailed  EvaluationState = "FAILED"
	EvaluationPaused  EvaluationState = "PAUSED"
)

// TargetOutcome is the normalized, persisted state of a canary member at the
// evaluation cut. The evidence adapter, not this pure evaluator, establishes
// enrollment and reservation truth.
type TargetOutcome struct {
	TargetID              string
	UpdateState           string
	ActiveEnrollment      bool
	UnresolvedReservation bool
}

// HealthSample identifies one scheduled health fact. Both schedule and
// observation time are required so a fresh unscheduled observation cannot
// authorize a release.
type HealthSample struct {
	TargetID    string
	Check       string
	ScheduledAt time.Time
	ObservedAt  time.Time
	Passed      bool
}

type EvaluationInput struct {
	Policy     Policy
	StartedAt  time.Time
	LogicalCut time.Time
	Outcomes   []TargetOutcome
	Health     []HealthSample
}

type EvaluationDecision struct {
	State     EvaluationState
	Reason    reason.Code
	Failed    uint64
	Evaluated uint64
}

// EvaluateCanary applies the persisted policy at an explicit logical cut. It
// contains no clock, store, or device access and fails closed on every missing
// fact. A PASSED result is the only result that can authorize the later wave.
func EvaluateCanary(input EvaluationInput) (EvaluationDecision, error) {
	if input.StartedAt.IsZero() || input.LogicalCut.IsZero() || input.LogicalCut.Before(input.StartedAt) || len(input.Outcomes) == 0 || input.Policy.Validate(uint64(len(input.Outcomes))) != nil {
		return EvaluationDecision{}, ErrInvalidEvaluationInput
	}
	if input.LogicalCut.Sub(input.StartedAt) > time.Duration(input.Policy.MaximumDurationSeconds)*time.Second {
		return EvaluationDecision{State: EvaluationPaused, Reason: reason.CodeCampaignCanaryDurationExpired}, nil
	}
	seen := make(map[string]struct{}, len(input.Outcomes))
	failed, evaluated := uint64(0), uint64(0)
	for _, outcome := range input.Outcomes {
		if outcome.TargetID == "" {
			return EvaluationDecision{}, ErrInvalidEvaluationInput
		}
		if _, duplicate := seen[outcome.TargetID]; duplicate {
			return EvaluationDecision{}, ErrInvalidEvaluationInput
		}
		seen[outcome.TargetID] = struct{}{}
		if !outcome.ActiveEnrollment || outcome.UnresolvedReservation {
			return EvaluationDecision{State: EvaluationPending, Reason: reason.CodeCampaignCanaryPending}, nil
		}
		switch outcome.UpdateState {
		case "SUCCEEDED":
			evaluated++
		case "FAILED_TERMINAL", "SKIPPED", "CANCELLED":
			evaluated++
			failed++
		case "NEEDS_ATTENTION":
			return EvaluationDecision{State: EvaluationPaused, Reason: reason.CodeCampaignCanaryNeedsAttention}, nil
		default:
			return EvaluationDecision{State: EvaluationPending, Reason: reason.CodeCampaignCanaryPending}, nil
		}
	}
	if evaluated != uint64(len(input.Outcomes)) {
		return EvaluationDecision{}, ErrInvalidEvaluationInput
	}
	if !healthyAtCut(input, seen) {
		return EvaluationDecision{State: EvaluationPending, Reason: reason.CodeCampaignCanaryHealthBlocked, Failed: failed, Evaluated: evaluated}, nil
	}
	if input.Policy.FailureBudget.Numerator > uint64(1<<63-1) || input.Policy.FailureBudget.Denominator > uint64(1<<63-1) {
		return EvaluationDecision{}, ErrInvalidEvaluationInput
	}
	budget, err := value.NewFailureBudget(int64(input.Policy.FailureBudget.Numerator), int64(input.Policy.FailureBudget.Denominator))
	if err != nil {
		return EvaluationDecision{}, ErrInvalidEvaluationInput
	}
	decision := policy.EvaluateFailureBudget(budget, failed, evaluated)
	if !decision.Valid {
		return EvaluationDecision{}, ErrInvalidEvaluationInput
	}
	if !decision.AdmissionAllowed {
		return EvaluationDecision{State: EvaluationFailed, Reason: reason.CodeCampaignBudgetThresholdReached, Failed: failed, Evaluated: evaluated}, nil
	}
	return EvaluationDecision{State: EvaluationPassed, Reason: reason.CodeCampaignCanaryPassed, Failed: failed, Evaluated: evaluated}, nil
}

func healthyAtCut(input EvaluationInput, targets map[string]struct{}) bool {
	samples := make(map[string]HealthSample, len(input.Health))
	for _, sample := range input.Health {
		if _, selected := targets[sample.TargetID]; !selected || sample.Check == "" || sample.ScheduledAt.IsZero() || sample.ObservedAt.IsZero() || sample.ScheduledAt.After(input.LogicalCut) || sample.ObservedAt.After(input.LogicalCut) || input.LogicalCut.Sub(sample.ObservedAt) > time.Duration(input.Policy.ObservationFreshSecond)*time.Second {
			return false
		}
		key := sample.TargetID + "\x00" + sample.Check
		if _, duplicate := samples[key]; duplicate || !sample.Passed {
			return false
		}
		samples[key] = sample
	}
	for target := range targets {
		for _, check := range input.Policy.RequiredHealthChecks {
			if _, ok := samples[target+"\x00"+check]; !ok {
				return false
			}
		}
	}
	return true
}
