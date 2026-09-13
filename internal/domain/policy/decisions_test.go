package policy

import (
	"testing"
	"time"

	"github.com/parth10-05/fleetforge/internal/domain/value"
)

func TestEvaluateFailureBudgetUsesInclusiveExactThreshold(t *testing.T) {
	budget, err := value.NewFailureBudget(1, 2)
	if err != nil {
		t.Fatal(err)
	}
	got := EvaluateFailureBudget(budget, 1, 2)
	if !got.Valid || got.AdmissionAllowed || !got.ThresholdReached {
		t.Fatalf("equality decision = %#v", got)
	}
	got = EvaluateFailureBudget(budget, 1, 3)
	if !got.Valid || !got.AdmissionAllowed || got.ThresholdReached {
		t.Fatalf("below decision = %#v", got)
	}
}

func TestEvaluateFailureBudgetFailsClosedForInvalidAggregate(t *testing.T) {
	if got := EvaluateFailureBudget(value.FailureBudget{}, 0, 0); got.Valid || got.AdmissionAllowed {
		t.Fatalf("zero denominator = %#v", got)
	}
	budget, _ := value.NewFailureBudget(1, 2)
	if got := EvaluateFailureBudget(budget, 2, 1); got.Valid || got.AdmissionAllowed {
		t.Fatalf("impossible counts = %#v", got)
	}
}

func TestClassifyRetryPreservesSafeResumeStage(t *testing.T) {
	budget, _ := value.NewRetryBudget(3)
	cases := []struct {
		name  string
		input RetryInput
		want  RetryDecision
	}{
		{"precheck", RetryInput{budget, 1, RetryStagePrecheck, RetryCausePrecheckTransient}, RetryDecision{RetryAllowed: true, ResumeStage: RetryStagePrecheck}},
		{"observation", RetryInput{budget, 1, RetryStageVerifying, RetryCauseObservationGap}, RetryDecision{RetryAllowed: true, ResumeStage: RetryStageVerifying}},
		{"ambiguous submit", RetryInput{budget, 1, RetryStageSubmitting, RetryCauseSubmissionAmbiguous}, RetryDecision{NeedsAttention: true}},
		{"exhausted", RetryInput{budget, 3, RetryStagePrecheck, RetryCausePrecheckTransient}, RetryDecision{}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := ClassifyRetry(testCase.input); got != testCase.want {
				t.Fatalf("ClassifyRetry() = %#v, want %#v", got, testCase.want)
			}
		})
	}
}

func TestEvaluateMaintenanceWindowHandlesDSTAndCrossMidnight(t *testing.T) {
	resolve := func(name string) (*time.Location, error) { return time.LoadLocation(name) }
	timezone, err := value.ParseTimezoneName("America/New_York", resolve)
	if err != nil {
		t.Fatal(err)
	}
	window, err := value.NewMaintenanceWindow(timezone, []time.Weekday{time.Saturday}, 23*60, 60)
	if err != nil {
		t.Fatal(err)
	}
	for _, testCase := range []struct {
		raw  string
		want bool
	}{
		{"2026-03-08T04:30:00Z", true},
		{"2026-03-08T06:00:00Z", true},
		{"2026-03-08T06:30:00Z", false},
		{"2026-03-08T07:30:00Z", false},
	} {
		now, _ := time.Parse(time.RFC3339, testCase.raw)
		timestamp, _ := value.NewUTCTimestamp(now)
		got := EvaluateMaintenanceWindow(window, timestamp, resolve)
		if !got.Valid || got.Allowed != testCase.want {
			t.Fatalf("%s decision = %#v, want allowed %t", testCase.raw, got, testCase.want)
		}
	}
}

func TestEvaluatePostUpdateSuccessRequiresFreshVersionAndHealth(t *testing.T) {
	expected, _ := value.ParseFirmwareVersion("v2")
	other, _ := value.ParseFirmwareVersion("v1")
	now, _ := value.NewUTCTimestamp(time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC))
	fresh, _ := value.NewUTCTimestamp(now.Time().Add(-time.Minute))
	stale, _ := value.NewUTCTimestamp(now.Time().Add(-6 * time.Minute))
	policy := SuccessPolicy{ExpectedVersion: expected, RequiredChecks: []string{"reachability", "inventory"}, MaximumAge: 5 * time.Minute}
	passing := []HealthObservation{{"reachability", true, fresh}, {"inventory", true, fresh}}
	if got := EvaluatePostUpdateSuccess(policy, expected, passing, now); !got.Valid || !got.Succeeded {
		t.Fatalf("passing decision = %#v", got)
	}
	if got := EvaluatePostUpdateSuccess(policy, other, passing, now); !got.Valid || got.Succeeded {
		t.Fatalf("wrong version = %#v", got)
	}
	if got := EvaluatePostUpdateSuccess(policy, expected, []HealthObservation{{"reachability", true, fresh}, {"inventory", true, stale}}, now); !got.Valid || got.Succeeded {
		t.Fatalf("stale health = %#v", got)
	}
}
