package policy

import "github.com/parth10-05/fleetforge/internal/domain/value"

// CampaignGuard names a policy fact that the campaign reducer requires for a
// specific transition. These names deliberately match the transition catalog,
// but callers cannot satisfy a transition with an unqualified Boolean.
type CampaignGuard string

const (
	CampaignGuardValidSnapshot          CampaignGuard = "valid_campaign_snapshot"
	CampaignGuardCancellationPermitted  CampaignGuard = "cancellation_permitted"
	CampaignGuardPreflightPassed        CampaignGuard = "preflight_passed"
	CampaignGuardSnapshotFrozen         CampaignGuard = "snapshot_frozen"
	CampaignGuardCorrectableFailure     CampaignGuard = "correctable_failure"
	CampaignGuardAuthorizedStart        CampaignGuard = "authorized_start"
	CampaignGuardCanaryTargetsTerminal  CampaignGuard = "canary_targets_terminal"
	CampaignGuardPauseAuthorized        CampaignGuard = "pause_authorized"
	CampaignGuardAbortAuthorized        CampaignGuard = "abort_authorized"
	CampaignGuardCanaryEvaluated        CampaignGuard = "canary_evaluated"
	CampaignGuardFailureBudgetPassed    CampaignGuard = "failure_budget_passed"
	CampaignGuardPriorWaveEvaluated     CampaignGuard = "prior_wave_evaluated"
	CampaignGuardAllTargetsTerminal     CampaignGuard = "all_targets_terminal"
	CampaignGuardFinalPolicyPassed      CampaignGuard = "final_policy_passed"
	CampaignGuardSuspendedPhasePresent  CampaignGuard = "suspended_phase_present"
	CampaignGuardInflightOutcomeBlocked CampaignGuard = "inflight_outcome_violates_policy"
	CampaignGuardRemainingWorkResolved  CampaignGuard = "remaining_work_resolved"
)

// CampaignAuthorization is an immutable snapshot of the named policy facts
// evaluated by the caller. It holds no transport, database, or clock state.
type CampaignAuthorization struct{ guards map[CampaignGuard]struct{} }

func NewCampaignAuthorization(guards ...CampaignGuard) CampaignAuthorization {
	approved := make(map[CampaignGuard]struct{}, len(guards))
	for _, guard := range guards {
		if knownCampaignGuard(guard) {
			approved[guard] = struct{}{}
		}
	}
	return CampaignAuthorization{guards: approved}
}

func (authorization CampaignAuthorization) Satisfies(required []string) bool {
	if len(required) == 0 || len(authorization.guards) == 0 {
		return false
	}
	for _, raw := range required {
		guard := CampaignGuard(raw)
		if !knownCampaignGuard(guard) {
			return false
		}
		if _, ok := authorization.guards[guard]; !ok {
			return false
		}
	}
	return true
}

func knownCampaignGuard(guard CampaignGuard) bool {
	switch guard {
	case CampaignGuardValidSnapshot, CampaignGuardCancellationPermitted, CampaignGuardPreflightPassed,
		CampaignGuardSnapshotFrozen, CampaignGuardCorrectableFailure, CampaignGuardAuthorizedStart,
		CampaignGuardCanaryTargetsTerminal, CampaignGuardPauseAuthorized, CampaignGuardAbortAuthorized,
		CampaignGuardCanaryEvaluated, CampaignGuardFailureBudgetPassed, CampaignGuardPriorWaveEvaluated,
		CampaignGuardAllTargetsTerminal, CampaignGuardFinalPolicyPassed, CampaignGuardSuspendedPhasePresent,
		CampaignGuardInflightOutcomeBlocked, CampaignGuardRemainingWorkResolved:
		return true
	default:
		return false
	}
}

// TargetAuthorization represents the independently evaluated policy decision
// required for every target transition.
type TargetAuthorization struct{ approved bool }

func ApproveTargetTransition() TargetAuthorization     { return TargetAuthorization{approved: true} }
func (authorization TargetAuthorization) Allows() bool { return authorization.approved }

// SubmissionAdmission proves that an admission was persisted for this exact
// command identity and epoch before the reducer may emit a submit intent.
type SubmissionAdmission struct {
	commandID value.CommandID
	epoch     value.CommandEpoch
}

func NewPersistedSubmissionAdmission(commandID value.CommandID, epoch value.CommandEpoch) SubmissionAdmission {
	return SubmissionAdmission{commandID: commandID, epoch: epoch}
}

func (admission SubmissionAdmission) Matches(commandID value.CommandID, epoch value.CommandEpoch) bool {
	return !admission.commandID.IsZero() && admission.commandID == commandID && admission.epoch == epoch
}

// IndependentAbsenceProof is available only when independent evidence proves
// the earlier call absent and excludes a late caller from recreating it.
type IndependentAbsenceProof struct{ proven bool }

func NewIndependentAbsenceAndLateCallerExclusion() IndependentAbsenceProof {
	return IndependentAbsenceProof{proven: true}
}

func (proof IndependentAbsenceProof) IsProven() bool { return proof.proven }

// VersionAndHealthSuccess proves both expected-version equality and all fresh
// required health checks. The policy evaluator owns deriving this value.
type VersionAndHealthSuccess struct{ verified bool }

func NewVersionAndHealthSuccess() VersionAndHealthSuccess {
	return VersionAndHealthSuccess{verified: true}
}
func (proof VersionAndHealthSuccess) IsVerified() bool { return proof.verified }
