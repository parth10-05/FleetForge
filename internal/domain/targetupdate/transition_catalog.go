package targetupdate

import "github.com/parth10-05/fleetforge/internal/domain/reason"

var catalog = buildCatalog()

func TransitionFor(state State, command Command) (Transition, bool) {
	transition, ok := catalog[state][command]
	return transition, ok
}
func AllTransitions() []Transition {
	result := make([]Transition, 0, len(AllStates())*len(AllCommands()))
	for _, state := range AllStates() {
		for _, command := range AllCommands() {
			transition, _ := TransitionFor(state, command)
			result = append(result, transition)
		}
	}
	return result
}
func buildCatalog() map[State]map[Command]Transition {
	result := map[State]map[Command]Transition{}
	for _, state := range AllStates() {
		result[state] = map[Command]Transition{}
		for _, command := range AllCommands() {
			result[state][command] = rejectedTransition(state, command)
		}
	}
	overrides := []Transition{
		allow(StatePending, CommandStartPrecheck, StatePrecheck, "", AttentionExitNone, reason.CodeTargetPrecheckIneligible, false), allow(StatePending, CommandCancelNoSideEffect, StateCancelled, "", AttentionExitNone, reason.CodeTargetPrecheckIneligible, false),
		allow(StatePrecheck, CommandPrecheckPassed, StateSubmitting, "", AttentionExitNone, reason.CodeTargetPrecheckIneligible, false), retry(StatePrecheck, CommandPrecheckRetryable, ResumeStepPrecheck), allow(StatePrecheck, CommandPrecheckTerminal, StateFailedTerminal, "", AttentionExitNone, reason.CodeTargetPrecheckIneligible, false), allow(StatePrecheck, CommandAlreadyCurrent, StateSucceeded, "", AttentionExitNone, reason.CodeTargetVerificationInsufficient, false), allow(StatePrecheck, CommandCancelNoSideEffect, StateCancelled, "", AttentionExitNone, reason.CodeTargetPrecheckIneligible, false),
		allow(StateSubmitting, CommandAdmitSubmission, StateSubmitting, "", AttentionExitNone, reason.CodeTargetSubmissionNotAuthorized, true), allow(StateSubmitting, CommandRemoteAccepted, StateUpdateAccepted, "", AttentionExitNone, reason.CodeTargetSubmissionNotAuthorized, false), retry(StateSubmitting, CommandPreSendAbsent, ResumeStepSubmitting), allow(StateSubmitting, CommandPostSendAmbiguous, StateNeedsAttention, "", AttentionExitNone, reason.CodeTargetAmbiguityInconclusive, false),
		allow(StateUpdateAccepted, CommandRebootObserved, StateWaitingReboot, "", AttentionExitNone, reason.CodeTargetVerificationInsufficient, false), allow(StateUpdateAccepted, CommandNoRebootRequired, StateVerifying, "", AttentionExitNone, reason.CodeTargetVerificationInsufficient, false), allow(StateUpdateAccepted, CommandContinueObservation, StateUpdateAccepted, "", AttentionExitNone, reason.CodeTargetAmbiguityInconclusive, false),
		allow(StateWaitingReboot, CommandRebootObserved, StateVerifying, "", AttentionExitNone, reason.CodeTargetVerificationInsufficient, false), retry(StateWaitingReboot, CommandVerificationRetryable, ResumeStepWaitingReboot), allow(StateWaitingReboot, CommandContinueObservation, StateWaitingReboot, "", AttentionExitNone, reason.CodeTargetAmbiguityInconclusive, false),
		allow(StateVerifying, CommandVerificationSucceeded, StateSucceeded, "", AttentionExitNone, reason.CodeTargetVerificationInsufficient, false), retry(StateVerifying, CommandVerificationRetryable, ResumeStepVerifying), allow(StateVerifying, CommandVerificationContradicted, StateFailedTerminal, "", AttentionExitNone, reason.CodeTargetVerificationInsufficient, false),
		allow(StateRetryWait, CommandRetryDue, StatePrecheck, ResumeStepPrecheck, AttentionExitNone, reason.CodeTargetPrecheckIneligible, false), allow(StateRetryWait, CommandAdmitSubmission, StateSubmitting, ResumeStepSubmitting, AttentionExitNone, reason.CodeTargetSubmissionNotAuthorized, true), allow(StateRetryWait, CommandRebootObserved, StateWaitingReboot, ResumeStepWaitingReboot, AttentionExitNone, reason.CodeTargetVerificationInsufficient, false), allow(StateRetryWait, CommandVerificationRetryable, StateVerifying, ResumeStepVerifying, AttentionExitNone, reason.CodeTargetVerificationInsufficient, false),
		allow(StateNeedsAttention, CommandAdoptOperation, StateUpdateAccepted, "", AttentionExitAdopt, reason.CodeTargetAmbiguityInconclusive, false), allow(StateNeedsAttention, CommandAttentionRetry, StateRetryWait, ResumeStepSubmitting, AttentionExitProofRetry, reason.CodeTargetAmbiguityInconclusive, false), allow(StateNeedsAttention, CommandAttentionTerminal, StateFailedTerminal, "", AttentionExitTerminal, reason.CodeTargetAmbiguityInconclusive, false), allow(StateNeedsAttention, CommandContinueObservation, StateNeedsAttention, "", AttentionExitNone, reason.CodeTargetAmbiguityInconclusive, false),
		noOp(StateSucceeded, CommandVerificationSucceeded), noOp(StateFailedTerminal, CommandVerificationContradicted), noOp(StateSkipped, CommandCancelNoSideEffect), noOp(StateCancelled, CommandCancelNoSideEffect),
	}
	for _, transition := range overrides {
		result[transition.From][transition.Command] = transition
	}
	return result
}
func rejectedTransition(from State, command Command) Transition {
	return Transition{From: from, Command: command, Class: TransitionRejected, Result: from, Reason: reason.CodeTargetPrecheckIneligible}
}
func allow(from State, command Command, result State, resume ResumeStep, attention AttentionExit, code reason.Code, submit bool) Transition {
	return Transition{From: from, Command: command, Class: TransitionAllowed, Result: result, Resume: resume, Attention: attention, Reason: code, SubmitIntent: submit}
}
func retry(from State, command Command, resume ResumeStep) Transition {
	return allow(from, command, StateRetryWait, resume, AttentionExitNone, reason.CodeTargetVerificationInsufficient, false)
}
func noOp(from State, command Command) Transition {
	return Transition{From: from, Command: command, Class: TransitionNoOp, Result: from, Reason: reason.CodeTargetVerificationInsufficient}
}
