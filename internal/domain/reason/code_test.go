package reason

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestCodeRegistryAcceptsEveryDeclaredCodeWithStableFamily(t *testing.T) {
	wantFamilies := map[Code]string{
		CodeCampaignValidationInvalid:         "campaign.validation",
		CodeCampaignValidationAccepted:        "campaign.validation",
		CodeCampaignValidationFailed:          "campaign.validation",
		CodeCampaignWaveBlocked:               "campaign.wave",
		CodeCampaignWaveAdvanced:              "campaign.wave",
		CodeCampaignBudgetThresholdReached:    "campaign.budget",
		CodeCampaignPauseEpochMismatch:        "campaign.pause",
		CodeCampaignPauseApplied:              "campaign.pause",
		CodeCampaignPauseResumed:              "campaign.pause",
		CodeCampaignPauseAbortStarted:         "campaign.pause",
		CodeCampaignPauseAbortResolved:        "campaign.pause",
		CodeCampaignPlanPolicyInvalid:         "campaign.plan",
		CodeCampaignPlanCanaryDiversity:       "campaign.plan",
		CodeCampaignPlanCanaryRankFill:        "campaign.plan",
		CodeCampaignPlanLaterWave:             "campaign.plan",
		CodeTargetPrecheckIneligible:          "target.precheck",
		CodeTargetSubmissionNotAuthorized:     "target.submission",
		CodeTargetAmbiguityInconclusive:       "target.ambiguity",
		CodeTargetVerificationInsufficient:    "target.verification",
		CodeArtifactVerificationFailed:        "artifact.verification",
		CodeConcurrencyReservationUnavailable: "concurrency.reservation",
		CodeInternalInvariantInvalidAggregate: "internal.invariant",
	}

	seen := make(map[Code]struct{}, len(wantFamilies))
	for _, want := range AllCodes() {
		if _, duplicate := seen[want]; duplicate {
			t.Fatalf("duplicate registered reason code %q", want)
		}
		seen[want] = struct{}{}
		got, err := ParseCode(want.String())
		if err != nil || got != want {
			t.Fatalf("ParseCode(%q) = %q, %v", want, got, err)
		}
		if got.Family() != wantFamilies[want] {
			t.Fatalf("Family(%q) = %q, want %q", got, got.Family(), wantFamilies[want])
		}
		encoded, err := json.Marshal(want)
		if err != nil {
			t.Fatalf("marshal %q: %v", want, err)
		}
		var raw string
		if err := json.Unmarshal(encoded, &raw); err != nil {
			t.Fatalf("unmarshal %q: %v", want, err)
		}
		if _, err := ParseCode(raw); err != nil {
			t.Fatalf("round-trip parse %q: %v", raw, err)
		}
	}
	if len(seen) != len(wantFamilies) {
		t.Fatalf("registered code count = %d, want %d", len(seen), len(wantFamilies))
	}
}

func TestCodeRegistryRejectsUnknownOrCaseChangedInput(t *testing.T) {
	for _, raw := range []string{"", "campaign.validation.future", strings.ToUpper(string(CodeCampaignValidationInvalid))} {
		_, err := ParseCode(raw)
		if !errors.Is(err, ErrUnknownCode) {
			t.Fatalf("ParseCode(%q) error = %v, want ErrUnknownCode", raw, err)
		}
	}
	codes := AllCodes()
	codes[0] = "mutated"
	if AllCodes()[0] != CodeCampaignValidationInvalid {
		t.Fatal("AllCodes exposed mutable registry storage")
	}
}
