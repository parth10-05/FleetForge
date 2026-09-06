package campaign

import "github.com/parth10-05/fleetforge/internal/domain/reason"

type Transition struct {
	From       State
	Command    Command
	Class      TransitionClass
	Result     State
	Resolution StateResolution
	Revision   RevisionEffect
	Epoch      EpochEffect
	Suspended  SuspendedPhaseEffect
	Guards     []string
	Reason     reason.Code
	Event      string
}

var transitionCatalog = buildTransitionCatalog()

func TransitionFor(state State, command Command) (Transition, bool) {
	transition, ok := transitionCatalog[state][command]
	if !ok {
		return Transition{}, false
	}
	transition.Guards = append([]string(nil), transition.Guards...)
	return transition, true
}

func AllTransitions() []Transition {
	transitions := make([]Transition, 0, len(AllStates())*len(AllCommands()))
	for _, state := range AllStates() {
		for _, command := range AllCommands() {
			transition, _ := TransitionFor(state, command)
			transitions = append(transitions, transition)
		}
	}
	return transitions
}

func buildTransitionCatalog() map[State]map[Command]Transition {
	catalog := make(map[State]map[Command]Transition, len(AllStates()))
	for _, state := range AllStates() {
		catalog[state] = make(map[Command]Transition, len(AllCommands()))
		for _, command := range AllCommands() {
			catalog[state][command] = rejectedTransition(state, command)
		}
	}
	overrides := []Transition{
		allowed(StateDraft, CommandBeginValidation, StateValidating, StateResolutionFixed, EpochUnchanged, SuspendedPhaseNone, reason.CodeCampaignValidationAccepted, "campaign.validation_started", "valid_campaign_snapshot"),
		allowed(StateDraft, CommandCancel, StateCancelled, StateResolutionFixed, EpochUnchanged, SuspendedPhaseNone, reason.CodeCampaignValidationAccepted, "campaign.cancelled", "cancellation_permitted"),
		allowed(StateValidating, CommandValidationPassed, StateReady, StateResolutionFixed, EpochUnchanged, SuspendedPhaseNone, reason.CodeCampaignValidationAccepted, "campaign.validation_succeeded", "preflight_passed", "snapshot_frozen"),
		allowed(StateValidating, CommandValidationFailed, StateDraft, StateResolutionFixed, EpochUnchanged, SuspendedPhaseNone, reason.CodeCampaignValidationFailed, "campaign.validation_failed", "correctable_failure"),
		noOp(StateValidating, CommandBeginValidation, reason.CodeCampaignValidationAccepted),
		allowed(StateReady, CommandStartCanary, StateRunningCanary, StateResolutionFixed, EpochUnchanged, SuspendedPhaseNone, reason.CodeCampaignWaveAdvanced, "campaign.canary_started", "authorized_start"),
		allowed(StateReady, CommandCancel, StateCancelled, StateResolutionFixed, EpochUnchanged, SuspendedPhaseNone, reason.CodeCampaignValidationAccepted, "campaign.cancelled", "cancellation_permitted"),
		allowed(StateRunningCanary, CommandCanaryTerminal, StateEvaluatingCanary, StateResolutionFixed, EpochUnchanged, SuspendedPhaseNone, reason.CodeCampaignWaveAdvanced, "campaign.canary_terminal", "canary_targets_terminal"),
		pause(StateRunningCanary),
		abort(StateRunningCanary),
		allowed(StateEvaluatingCanary, CommandApproveCanary, StateRunningWaves, StateResolutionFixed, EpochUnchanged, SuspendedPhaseNone, reason.CodeCampaignWaveAdvanced, "campaign.canary_approved", "canary_evaluated", "failure_budget_passed"),
		pause(StateEvaluatingCanary), abort(StateEvaluatingCanary),
		allowed(StateRunningWaves, CommandReleaseNextWave, StateRunningWaves, StateResolutionFixed, EpochUnchanged, SuspendedPhaseNone, reason.CodeCampaignWaveAdvanced, "campaign.wave_released", "prior_wave_evaluated"),
		allowed(StateRunningWaves, CommandComplete, StateCompleted, StateResolutionFixed, EpochUnchanged, SuspendedPhaseNone, reason.CodeCampaignWaveAdvanced, "campaign.completed", "all_targets_terminal", "final_policy_passed"),
		pause(StateRunningWaves), abort(StateRunningWaves),
		allowed(StatePaused, CommandResume, "", StateResolutionRestoreSuspended, EpochUnchanged, SuspendedPhaseRestore, reason.CodeCampaignPauseResumed, "campaign.resumed", "suspended_phase_present"),
		noOp(StatePaused, CommandPause, reason.CodeCampaignPauseApplied),
		abort(StatePaused),
		allowed(StateAborting, CommandAbortFailed, StateFailed, StateResolutionFixed, EpochUnchanged, SuspendedPhaseNone, reason.CodeCampaignPauseAbortResolved, "campaign.abort_failed", "inflight_outcome_violates_policy"),
		allowed(StateAborting, CommandAbortCancelled, StateCancelled, StateResolutionFixed, EpochUnchanged, SuspendedPhaseNone, reason.CodeCampaignPauseAbortResolved, "campaign.abort_cancelled", "remaining_work_resolved"),
		noOp(StateCompleted, CommandComplete, reason.CodeCampaignWaveAdvanced), noOp(StateFailed, CommandAbortFailed, reason.CodeCampaignPauseAbortResolved), noOp(StateCancelled, CommandCancel, reason.CodeCampaignValidationAccepted),
	}
	for _, transition := range overrides {
		catalog[transition.From][transition.Command] = transition
	}
	return catalog
}

func rejectedTransition(state State, command Command) Transition {
	return Transition{From: state, Command: command, Class: TransitionRejected, Result: state, Resolution: StateResolutionFixed, Revision: RevisionUnchanged, Epoch: EpochUnchanged, Suspended: SuspendedPhaseNone, Reason: reason.CodeCampaignValidationInvalid}
}
func allowed(from State, command Command, result State, resolution StateResolution, epoch EpochEffect, suspended SuspendedPhaseEffect, code reason.Code, event string, guards ...string) Transition {
	return Transition{From: from, Command: command, Class: TransitionAllowed, Result: result, Resolution: resolution, Revision: RevisionIncrement, Epoch: epoch, Suspended: suspended, Guards: guards, Reason: code, Event: event}
}
func noOp(from State, command Command, code reason.Code) Transition {
	return Transition{From: from, Command: command, Class: TransitionNoOp, Result: from, Resolution: StateResolutionFixed, Revision: RevisionUnchanged, Epoch: EpochUnchanged, Suspended: SuspendedPhaseNone, Reason: code}
}
func pause(from State) Transition {
	return allowed(from, CommandPause, StatePaused, StateResolutionFixed, EpochIncrement, SuspendedPhaseCapture, reason.CodeCampaignPauseApplied, "campaign.paused", "pause_authorized")
}
func abort(from State) Transition {
	return allowed(from, CommandAbort, StateAborting, StateResolutionFixed, EpochIncrement, SuspendedPhaseNone, reason.CodeCampaignPauseAbortStarted, "campaign.abort_started", "abort_authorized")
}
