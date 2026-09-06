package reason

import "errors"

var ErrUnknownCode = errors.New("unknown domain reason code")

// Code is a stable machine-readable explanation for a domain decision. Human
// presentation is deliberately owned outside the domain layer.
type Code string

const (
	CodeCampaignValidationInvalid         Code = "campaign.validation.invalid"
	CodeCampaignValidationAccepted        Code = "campaign.validation.accepted"
	CodeCampaignValidationFailed          Code = "campaign.validation.failed"
	CodeCampaignWaveBlocked               Code = "campaign.wave.blocked"
	CodeCampaignWaveAdvanced              Code = "campaign.wave.advanced"
	CodeCampaignBudgetThresholdReached    Code = "campaign.budget.threshold_reached"
	CodeCampaignPauseEpochMismatch        Code = "campaign.pause.command_epoch_mismatch"
	CodeCampaignPauseApplied              Code = "campaign.pause.applied"
	CodeCampaignPauseResumed              Code = "campaign.pause.resumed"
	CodeCampaignPauseAbortStarted         Code = "campaign.pause.abort_started"
	CodeCampaignPauseAbortResolved        Code = "campaign.pause.abort_resolved"
	CodeTargetPrecheckIneligible          Code = "target.precheck.ineligible"
	CodeTargetSubmissionNotAuthorized     Code = "target.submission.not_authorized"
	CodeTargetAmbiguityInconclusive       Code = "target.ambiguity.inconclusive"
	CodeTargetVerificationInsufficient    Code = "target.verification.insufficient_evidence"
	CodeArtifactVerificationFailed        Code = "artifact.verification.failed"
	CodeConcurrencyReservationUnavailable Code = "concurrency.reservation.unavailable"
	CodeInternalInvariantInvalidAggregate Code = "internal.invariant.invalid_aggregate"
)

var allCodes = []Code{
	CodeCampaignValidationInvalid,
	CodeCampaignValidationAccepted,
	CodeCampaignValidationFailed,
	CodeCampaignWaveBlocked,
	CodeCampaignWaveAdvanced,
	CodeCampaignBudgetThresholdReached,
	CodeCampaignPauseEpochMismatch,
	CodeCampaignPauseApplied,
	CodeCampaignPauseResumed,
	CodeCampaignPauseAbortStarted,
	CodeCampaignPauseAbortResolved,
	CodeTargetPrecheckIneligible,
	CodeTargetSubmissionNotAuthorized,
	CodeTargetAmbiguityInconclusive,
	CodeTargetVerificationInsufficient,
	CodeArtifactVerificationFailed,
	CodeConcurrencyReservationUnavailable,
	CodeInternalInvariantInvalidAggregate,
}

// ParseCode accepts only a registered code. New domain behaviors must add a
// named constant and registry entry deliberately rather than inventing text at
// a reducer call site.
func ParseCode(raw string) (Code, error) {
	code := Code(raw)
	for _, candidate := range allCodes {
		if code == candidate {
			return code, nil
		}
	}
	return "", ErrUnknownCode
}

func (code Code) String() string { return string(code) }

// Family returns the registered stable family prefix for a reason code.
func (code Code) Family() string {
	for _, candidate := range allCodes {
		if code == candidate {
			raw := string(code)
			for index := len(raw) - 1; index >= 0; index-- {
				if raw[index] == '.' {
					return raw[:index]
				}
			}
		}
	}
	return ""
}

func AllCodes() []Code { return append([]Code(nil), allCodes...) }
