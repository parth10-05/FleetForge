// Package policy contains pure, deterministic rollout-safety decisions.
package policy

import (
	"math/big"
	"time"

	"github.com/parth10-05/fleetforge/internal/domain/value"
)

// FailureBudgetDecision evaluates terminal outcomes only. A zero denominator
// or more failed than evaluated outcomes is invalid and therefore blocks new
// admissions. The threshold is inclusive: equality reaches the budget.
type FailureBudgetDecision struct {
	Valid            bool
	AdmissionAllowed bool
	ThresholdReached bool
}

func EvaluateFailureBudget(budget value.FailureBudget, failed, evaluated uint64) FailureBudgetDecision {
	if budget.Denominator() == 0 || failed > evaluated {
		return FailureBudgetDecision{}
	}
	if evaluated == 0 || failed == 0 {
		return FailureBudgetDecision{Valid: true, AdmissionAllowed: true}
	}

	left := new(big.Int).Mul(new(big.Int).SetUint64(failed), new(big.Int).SetUint64(budget.Denominator()))
	right := new(big.Int).Mul(new(big.Int).SetUint64(budget.Numerator()), new(big.Int).SetUint64(evaluated))
	reached := left.Cmp(right) >= 0
	return FailureBudgetDecision{Valid: true, AdmissionAllowed: !reached, ThresholdReached: reached}
}

type RetryCause string

const (
	RetryCausePrecheckTransient   RetryCause = "PRECHECK_TRANSIENT"
	RetryCauseSubmissionRejected  RetryCause = "SUBMISSION_REJECTED"
	RetryCauseObservationGap      RetryCause = "OBSERVATION_GAP"
	RetryCauseSubmissionAmbiguous RetryCause = "SUBMISSION_AMBIGUOUS"
	RetryCauseTerminal            RetryCause = "TERMINAL"
)

type RetryStage string

const (
	RetryStagePrecheck      RetryStage = "PRECHECK"
	RetryStageSubmitting    RetryStage = "SUBMITTING"
	RetryStageWaitingReboot RetryStage = "WAITING_REBOOT"
	RetryStageVerifying     RetryStage = "VERIFYING"
)

// RetryInput has no retry delay: orchestration owns scheduling. AttemptsUsed
// counts the completed or admitted attempts for the supplied stage.
type RetryInput struct {
	Budget       value.RetryBudget
	AttemptsUsed uint64
	Stage        RetryStage
	Cause        RetryCause
}

type RetryDecision struct {
	RetryAllowed   bool
	NeedsAttention bool
	ResumeStage    RetryStage
}

// ClassifyRetry never turns a possibly accepted submission into permission to
// submit again. Observation gaps can resume only observation stages.
func ClassifyRetry(input RetryInput) RetryDecision {
	if input.Budget.MaxAttempts() == 0 || input.AttemptsUsed >= input.Budget.MaxAttempts() {
		return RetryDecision{}
	}
	switch input.Cause {
	case RetryCausePrecheckTransient:
		if input.Stage == RetryStagePrecheck {
			return RetryDecision{RetryAllowed: true, ResumeStage: RetryStagePrecheck}
		}
	case RetryCauseSubmissionRejected:
		if input.Stage == RetryStageSubmitting {
			return RetryDecision{RetryAllowed: true, ResumeStage: RetryStageSubmitting}
		}
	case RetryCauseObservationGap:
		if input.Stage == RetryStageWaitingReboot || input.Stage == RetryStageVerifying {
			return RetryDecision{RetryAllowed: true, ResumeStage: input.Stage}
		}
	case RetryCauseSubmissionAmbiguous:
		return RetryDecision{NeedsAttention: true}
	}
	return RetryDecision{}
}

type MaintenanceDecision struct {
	Valid   bool
	Allowed bool
}

// EvaluateMaintenanceWindow evaluates an injected UTC instant in the window's
// IANA timezone. Both endpoints are included. For a cross-midnight window,
// minutes after midnight belong to the previous selected weekday.
func EvaluateMaintenanceWindow(window value.MaintenanceWindow, now value.UTCTimestamp, resolve value.TimezoneResolver) MaintenanceDecision {
	if now.IsZero() {
		return MaintenanceDecision{}
	}
	location, err := window.Timezone().Resolve(resolve)
	if err != nil || location == nil {
		return MaintenanceDecision{}
	}
	local := now.Time().In(location)
	minute := local.Hour()*60 + local.Minute()
	start, end := window.StartMinute(), window.EndMinute()
	if start < end {
		return MaintenanceDecision{Valid: true, Allowed: window.IncludesWeekday(local.Weekday()) && minute >= start && minute <= end}
	}
	if minute >= start {
		return MaintenanceDecision{Valid: true, Allowed: window.IncludesWeekday(local.Weekday())}
	}
	return MaintenanceDecision{Valid: true, Allowed: minute <= end && window.IncludesWeekday(local.AddDate(0, 0, -1).Weekday())}
}

type HealthObservation struct {
	Name       string
	Passed     bool
	ObservedAt value.UTCTimestamp
}

type SuccessPolicy struct {
	ExpectedVersion value.FirmwareVersion
	RequiredChecks  []string
	MaximumAge      time.Duration
}

type SuccessDecision struct {
	Valid     bool
	Succeeded bool
}

// EvaluatePostUpdateSuccess succeeds only when the expected version and each
// uniquely configured, fresh health check are positively evidenced.
func EvaluatePostUpdateSuccess(policy SuccessPolicy, observed value.FirmwareVersion, checks []HealthObservation, now value.UTCTimestamp) SuccessDecision {
	if policy.ExpectedVersion.IsZero() || now.IsZero() || policy.MaximumAge < 0 || observed != policy.ExpectedVersion {
		return SuccessDecision{Valid: !policy.ExpectedVersion.IsZero() && !now.IsZero() && policy.MaximumAge >= 0}
	}
	required := make(map[string]struct{}, len(policy.RequiredChecks))
	for _, name := range policy.RequiredChecks {
		if name == "" {
			return SuccessDecision{}
		}
		if _, duplicate := required[name]; duplicate {
			return SuccessDecision{}
		}
		required[name] = struct{}{}
	}
	observedChecks := make(map[string]HealthObservation, len(checks))
	for _, check := range checks {
		if check.Name == "" || check.ObservedAt.IsZero() {
			return SuccessDecision{}
		}
		if _, duplicate := observedChecks[check.Name]; duplicate {
			return SuccessDecision{}
		}
		observedChecks[check.Name] = check
	}
	for name := range required {
		check, exists := observedChecks[name]
		if !exists || !check.Passed || check.ObservedAt.Time().After(now.Time()) || now.Time().Sub(check.ObservedAt.Time()) > policy.MaximumAge {
			return SuccessDecision{Valid: true}
		}
	}
	return SuccessDecision{Valid: true, Succeeded: true}
}
