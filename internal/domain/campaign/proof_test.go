package campaign

import "github.com/parth10-05/fleetforge/internal/domain/policy"

func authorizedCampaign() policy.CampaignAuthorization {
	return policy.NewCampaignAuthorization(
		policy.CampaignGuardValidSnapshot, policy.CampaignGuardCancellationPermitted,
		policy.CampaignGuardPreflightPassed, policy.CampaignGuardSnapshotFrozen,
		policy.CampaignGuardCorrectableFailure, policy.CampaignGuardAuthorizedStart,
		policy.CampaignGuardCanaryTargetsTerminal, policy.CampaignGuardPauseAuthorized,
		policy.CampaignGuardAbortAuthorized, policy.CampaignGuardCanaryEvaluated,
		policy.CampaignGuardFailureBudgetPassed, policy.CampaignGuardPriorWaveEvaluated,
		policy.CampaignGuardAllTargetsTerminal, policy.CampaignGuardFinalPolicyPassed,
		policy.CampaignGuardSuspendedPhasePresent, policy.CampaignGuardInflightOutcomeBlocked,
		policy.CampaignGuardRemainingWorkResolved,
	)
}
