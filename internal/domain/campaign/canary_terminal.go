package campaign

import (
	"github.com/parth10-05/fleetforge/internal/domain/policy"
	"github.com/parth10-05/fleetforge/internal/domain/value"
)

// CanaryTerminalTransition is the two-step reducer result required after a
// terminal canary evaluation. Keeping both reductions here prevents a
// persistence adapter from skipping EVALUATING_CANARY on the way to either a
// release or a pause.
type CanaryTerminalTransition struct {
	Terminal Decision
	Resolved Decision
}

// ReduceTerminalCanary applies CANARY_TERMINAL first and then either APPROVE_CANARY
// (for the sole later-wave release) or PAUSE (for every fail-closed outcome).
// It performs no I/O and accepts only the reducer's explicit policy guards.
func ReduceTerminalCanary(current Aggregate, terminalID, resolutionID value.CommandID, release bool) (CanaryTerminalTransition, bool) {
	terminal := Reduce(current, CommandInput{ID: terminalID, Command: CommandCanaryTerminal, ExpectedRevision: current.Revision, ExpectedEpoch: current.CommandEpoch, Authorization: policy.NewCampaignAuthorization(policy.CampaignGuardCanaryTargetsTerminal)})
	if terminal.Class != DecisionAllowed {
		return CanaryTerminalTransition{}, false
	}
	if release {
		resolved := Reduce(terminal.Aggregate, CommandInput{ID: resolutionID, Command: CommandApproveCanary, ExpectedRevision: terminal.Aggregate.Revision, ExpectedEpoch: terminal.Aggregate.CommandEpoch, Authorization: policy.NewCampaignAuthorization(policy.CampaignGuardCanaryEvaluated, policy.CampaignGuardFailureBudgetPassed)})
		return CanaryTerminalTransition{Terminal: terminal, Resolved: resolved}, resolved.Class == DecisionAllowed
	}
	resolved := Reduce(terminal.Aggregate, CommandInput{ID: resolutionID, Command: CommandPause, ExpectedRevision: terminal.Aggregate.Revision, ExpectedEpoch: terminal.Aggregate.CommandEpoch, Authorization: policy.NewCampaignAuthorization(policy.CampaignGuardPauseAuthorized)})
	return CanaryTerminalTransition{Terminal: terminal, Resolved: resolved}, resolved.Class == DecisionAllowed
}
