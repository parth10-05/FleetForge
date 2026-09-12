package model

type TargetState string
type TargetCommand string
type TargetResumeStep string

const (
	TargetPending                  TargetState      = "PENDING"
	TargetPrecheck                 TargetState      = "PRECHECK"
	TargetSubmitting               TargetState      = "SUBMITTING"
	TargetUpdateAccepted           TargetState      = "UPDATE_ACCEPTED"
	TargetWaitingReboot            TargetState      = "WAITING_REBOOT"
	TargetVerifying                TargetState      = "VERIFYING"
	TargetRetryWait                TargetState      = "RETRY_WAIT"
	TargetNeedsAttention           TargetState      = "NEEDS_ATTENTION"
	TargetSucceeded                TargetState      = "SUCCEEDED"
	TargetFailedTerminal           TargetState      = "FAILED_TERMINAL"
	TargetSkipped                  TargetState      = "SKIPPED"
	TargetCancelled                TargetState      = "CANCELLED"
	TargetResumePrecheck           TargetResumeStep = "PRECHECK"
	TargetResumeSubmitting         TargetResumeStep = "SUBMITTING"
	TargetResumeWaitingReboot      TargetResumeStep = "WAITING_REBOOT"
	TargetResumeVerifying          TargetResumeStep = "VERIFYING"
	TargetStartPrecheck            TargetCommand    = "START_PRECHECK"
	TargetPrecheckPassed           TargetCommand    = "PRECHECK_PASSED"
	TargetPrecheckRetryable        TargetCommand    = "PRECHECK_RETRYABLE"
	TargetPrecheckTerminal         TargetCommand    = "PRECHECK_TERMINAL"
	TargetAlreadyCurrent           TargetCommand    = "ALREADY_CURRENT"
	TargetAdmitSubmission          TargetCommand    = "ADMIT_SUBMISSION"
	TargetRemoteAccepted           TargetCommand    = "REMOTE_ACCEPTED"
	TargetPreSendAbsent            TargetCommand    = "PRE_SEND_PROVEN_ABSENT"
	TargetPostSendAmbiguous        TargetCommand    = "POST_SEND_AMBIGUOUS"
	TargetRebootObserved           TargetCommand    = "REBOOT_OBSERVED"
	TargetNoRebootRequired         TargetCommand    = "NO_REBOOT_REQUIRED"
	TargetVerificationSucceeded    TargetCommand    = "VERIFICATION_SUCCEEDED"
	TargetVerificationRetryable    TargetCommand    = "VERIFICATION_RETRYABLE"
	TargetVerificationContradicted TargetCommand    = "VERIFICATION_CONTRADICTED"
	TargetRetryDue                 TargetCommand    = "RETRY_DUE"
	TargetAdoptOperation           TargetCommand    = "ADOPT_OPERATION"
	TargetAttentionRetry           TargetCommand    = "ATTENTION_RETRY_PROVEN_ABSENT"
	TargetAttentionTerminal        TargetCommand    = "ATTENTION_TERMINAL_FAILURE"
	TargetCancelNoSideEffect       TargetCommand    = "CANCEL_NO_SIDE_EFFECT"
	TargetContinueObservation      TargetCommand    = "CONTINUE_OBSERVATION"
)

type TargetAggregate struct {
	State      TargetState
	Revision   uint64
	ResumeStep TargetResumeStep
}

// StepTarget is handwritten and independent from targetupdate's catalog. It
// returns submit only for an explicit admission transition.
func StepTarget(current TargetAggregate, command TargetCommand) (TargetAggregate, bool, bool) {
	advance := func(state TargetState, resume TargetResumeStep, submit bool) (TargetAggregate, bool, bool) {
		next := TargetAggregate{State: state, Revision: current.Revision + 1}
		if state == TargetRetryWait {
			next.ResumeStep = resume
		}
		return next, true, submit
	}
	switch current.State {
	case TargetPending:
		switch command {
		case TargetStartPrecheck:
			return advance(TargetPrecheck, "", false)
		case TargetCancelNoSideEffect:
			return advance(TargetCancelled, "", false)
		}
	case TargetPrecheck:
		switch command {
		case TargetPrecheckPassed:
			return advance(TargetSubmitting, "", false)
		case TargetPrecheckRetryable:
			return advance(TargetRetryWait, TargetResumePrecheck, false)
		case TargetPrecheckTerminal:
			return advance(TargetFailedTerminal, "", false)
		case TargetAlreadyCurrent:
			return advance(TargetSucceeded, "", false)
		case TargetCancelNoSideEffect:
			return advance(TargetCancelled, "", false)
		}
	case TargetSubmitting:
		switch command {
		case TargetAdmitSubmission:
			return advance(TargetSubmitting, "", true)
		case TargetRemoteAccepted:
			return advance(TargetUpdateAccepted, "", false)
		case TargetPreSendAbsent:
			return advance(TargetRetryWait, TargetResumeSubmitting, false)
		case TargetPostSendAmbiguous:
			return advance(TargetNeedsAttention, "", false)
		}
	case TargetUpdateAccepted:
		switch command {
		case TargetRebootObserved:
			return advance(TargetWaitingReboot, "", false)
		case TargetNoRebootRequired:
			return advance(TargetVerifying, "", false)
		case TargetContinueObservation:
			return advance(TargetUpdateAccepted, "", false)
		}
	case TargetWaitingReboot:
		switch command {
		case TargetRebootObserved:
			return advance(TargetVerifying, "", false)
		case TargetVerificationRetryable:
			return advance(TargetRetryWait, TargetResumeWaitingReboot, false)
		case TargetContinueObservation:
			return advance(TargetWaitingReboot, "", false)
		}
	case TargetVerifying:
		switch command {
		case TargetVerificationSucceeded:
			return advance(TargetSucceeded, "", false)
		case TargetVerificationRetryable:
			return advance(TargetRetryWait, TargetResumeVerifying, false)
		case TargetVerificationContradicted:
			return advance(TargetFailedTerminal, "", false)
		}
	case TargetRetryWait:
		switch command {
		case TargetRetryDue:
			return advance(TargetPrecheck, "", false)
		case TargetAdmitSubmission:
			return advance(TargetSubmitting, "", true)
		case TargetRebootObserved:
			return advance(TargetWaitingReboot, "", false)
		case TargetVerificationRetryable:
			return advance(TargetVerifying, "", false)
		}
	case TargetNeedsAttention:
		switch command {
		case TargetAdoptOperation:
			return advance(TargetUpdateAccepted, "", false)
		case TargetAttentionRetry:
			return advance(TargetRetryWait, TargetResumeSubmitting, false)
		case TargetAttentionTerminal:
			return advance(TargetFailedTerminal, "", false)
		case TargetContinueObservation:
			return advance(TargetNeedsAttention, "", false)
		}
	}
	return current, false, false
}
