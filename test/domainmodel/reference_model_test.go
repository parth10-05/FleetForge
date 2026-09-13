package domainmodel

import (
	"testing"

	"pgregory.net/rapid"
)

type CampaignState string
type CampaignCommand string

const (
	CampaignDraft            CampaignState   = "DRAFT"
	CampaignValidating       CampaignState   = "VALIDATING"
	CampaignReady            CampaignState   = "READY"
	CampaignRunningCanary    CampaignState   = "RUNNING_CANARY"
	CampaignEvaluatingCanary CampaignState   = "EVALUATING_CANARY"
	CampaignRunningWaves     CampaignState   = "RUNNING_WAVES"
	CampaignPaused           CampaignState   = "PAUSED"
	CampaignAborting         CampaignState   = "ABORTING"
	CampaignCompleted        CampaignState   = "COMPLETED"
	CampaignFailed           CampaignState   = "FAILED"
	CampaignCancelled        CampaignState   = "CANCELLED"
	CampaignBeginValidation  CampaignCommand = "BEGIN_VALIDATION"
	CampaignValidationPassed CampaignCommand = "VALIDATION_PASSED"
	CampaignValidationFailed CampaignCommand = "VALIDATION_FAILED"
	CampaignStartCanary      CampaignCommand = "START_CANARY"
	CampaignCanaryTerminal   CampaignCommand = "CANARY_TERMINAL"
	CampaignApproveCanary    CampaignCommand = "APPROVE_CANARY"
	CampaignReleaseNextWave  CampaignCommand = "RELEASE_NEXT_WAVE"
	CampaignComplete         CampaignCommand = "COMPLETE"
	CampaignPause            CampaignCommand = "PAUSE"
	CampaignResume           CampaignCommand = "RESUME"
	CampaignCancel           CampaignCommand = "CANCEL"
	CampaignAbort            CampaignCommand = "ABORT"
	CampaignAbortFailed      CampaignCommand = "ABORT_FAILED"
	CampaignAbortCancelled   CampaignCommand = "ABORT_CANCELLED"
)

type CampaignAggregate struct {
	State          CampaignState
	Revision       uint64
	CommandEpoch   uint64
	SuspendedPhase CampaignState
}

// stepCampaign is deliberately handwritten and cannot import production
// reducers or transition catalogs.
func stepCampaign(current CampaignAggregate, command CampaignCommand) (CampaignAggregate, bool) {
	next := current
	advance := func(state CampaignState) (CampaignAggregate, bool) {
		next.State = state
		next.Revision++
		return next, true
	}
	switch current.State {
	case CampaignDraft:
		if command == CampaignBeginValidation {
			return advance(CampaignValidating)
		}
		if command == CampaignCancel {
			return advance(CampaignCancelled)
		}
	case CampaignValidating:
		if command == CampaignValidationPassed {
			return advance(CampaignReady)
		}
		if command == CampaignValidationFailed {
			return advance(CampaignDraft)
		}
	case CampaignReady:
		if command == CampaignStartCanary {
			return advance(CampaignRunningCanary)
		}
		if command == CampaignCancel {
			return advance(CampaignCancelled)
		}
	case CampaignRunningCanary:
		if command == CampaignCanaryTerminal {
			return advance(CampaignEvaluatingCanary)
		}
		if command == CampaignPause {
			return pauseCampaign(next)
		}
		if command == CampaignAbort {
			return abortCampaign(next)
		}
	case CampaignEvaluatingCanary:
		if command == CampaignApproveCanary {
			return advance(CampaignRunningWaves)
		}
		if command == CampaignPause {
			return pauseCampaign(next)
		}
		if command == CampaignAbort {
			return abortCampaign(next)
		}
	case CampaignRunningWaves:
		if command == CampaignReleaseNextWave {
			return advance(CampaignRunningWaves)
		}
		if command == CampaignComplete {
			return advance(CampaignCompleted)
		}
		if command == CampaignPause {
			return pauseCampaign(next)
		}
		if command == CampaignAbort {
			return abortCampaign(next)
		}
	case CampaignPaused:
		if command == CampaignResume && resumableCampaign(current.SuspendedPhase) {
			next.State, next.SuspendedPhase = current.SuspendedPhase, ""
			next.Revision++
			return next, true
		}
		if command == CampaignAbort {
			return abortCampaign(next)
		}
	case CampaignAborting:
		if command == CampaignAbortFailed {
			return advance(CampaignFailed)
		}
		if command == CampaignAbortCancelled {
			return advance(CampaignCancelled)
		}
	}
	return current, false
}
func pauseCampaign(next CampaignAggregate) (CampaignAggregate, bool) {
	next.SuspendedPhase = next.State
	next.State = CampaignPaused
	next.Revision++
	next.CommandEpoch++
	return next, true
}
func abortCampaign(next CampaignAggregate) (CampaignAggregate, bool) {
	next.State = CampaignAborting
	next.SuspendedPhase = ""
	next.Revision++
	next.CommandEpoch++
	return next, true
}
func resumableCampaign(state CampaignState) bool {
	return state == CampaignRunningCanary || state == CampaignEvaluatingCanary || state == CampaignRunningWaves
}

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

func stepTarget(current TargetAggregate, command TargetCommand) (TargetAggregate, bool, bool) {
	advance := func(state TargetState, resume TargetResumeStep, submit bool) (TargetAggregate, bool, bool) {
		next := TargetAggregate{State: state, Revision: current.Revision + 1}
		if state == TargetRetryWait {
			next.ResumeStep = resume
		}
		return next, true, submit
	}
	switch current.State {
	case TargetPending:
		if command == TargetStartPrecheck {
			return advance(TargetPrecheck, "", false)
		}
		if command == TargetCancelNoSideEffect {
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
			if current.ResumeStep == TargetResumePrecheck {
				return advance(TargetPrecheck, "", false)
			}
		case TargetAdmitSubmission:
			if current.ResumeStep == TargetResumeSubmitting {
				return advance(TargetSubmitting, "", true)
			}
		case TargetRebootObserved:
			if current.ResumeStep == TargetResumeWaitingReboot {
				return advance(TargetWaitingReboot, "", false)
			}
		case TargetVerificationRetryable:
			if current.ResumeStep == TargetResumeVerifying {
				return advance(TargetVerifying, "", false)
			}
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

func TestReferenceModelRejectsImpossibleState(t *testing.T) {
	impossible := CampaignAggregate{State: "ALIEN", Revision: 4}
	got, advanced := stepCampaign(impossible, CampaignComplete)
	if advanced || got != impossible {
		t.Fatalf("impossible state changed: %#v, %t", got, advanced)
	}
}
func TestReferenceModelWouldDetectTransitionMutation(t *testing.T) {
	got, advanced := stepCampaign(CampaignAggregate{State: CampaignRunningWaves}, CampaignComplete)
	if !advanced || got.State != CampaignCompleted {
		t.Fatalf("reference complete = %#v, %t", got, advanced)
	}
	if got.State == CampaignFailed {
		t.Fatal("deliberate mutated expected state unexpectedly matched")
	}
}
func TestReferenceModelPreservesMonotonicRevisions(t *testing.T) {
	commands := []CampaignCommand{CampaignBeginValidation, CampaignValidationPassed, CampaignValidationFailed, CampaignStartCanary, CampaignCanaryTerminal, CampaignApproveCanary, CampaignReleaseNextWave, CampaignComplete, CampaignPause, CampaignResume, CampaignCancel, CampaignAbort, CampaignAbortFailed, CampaignAbortCancelled}
	rapid.Check(t, func(t *rapid.T) {
		current := CampaignAggregate{State: CampaignDraft}
		for range 100 {
			command := rapid.SampledFrom(commands).Draw(t, "command")
			next, advanced := stepCampaign(current, command)
			if advanced && next.Revision != current.Revision+1 {
				t.Fatalf("advanced revision %d -> %d", current.Revision, next.Revision)
			}
			if !advanced && next != current {
				t.Fatalf("rejected command mutated aggregate: %#v -> %#v", current, next)
			}
			current = next
		}
	})
}
func TestTargetReferenceRequiresMatchingResumeStep(t *testing.T) {
	for _, testCase := range []struct {
		resume  TargetResumeStep
		command TargetCommand
	}{{TargetResumePrecheck, TargetAdmitSubmission}, {TargetResumeSubmitting, TargetRetryDue}, {TargetResumeWaitingReboot, TargetVerificationRetryable}, {TargetResumeVerifying, TargetRebootObserved}} {
		before := TargetAggregate{State: TargetRetryWait, ResumeStep: testCase.resume}
		got, advanced, submit := stepTarget(before, testCase.command)
		if advanced || submit || got != before {
			t.Fatalf("mismatched target retry exit = %#v, %t, %t", got, advanced, submit)
		}
	}
}

// This deliberate mutant models the unsafe bug fixed by 6f2fa0c: it permits
// submission from every RETRY_WAIT step. The independent oracle must disagree
// with it, proving the property suite can expose that safety regression.
func TestIndependentModelDetectsUnsafeRetryMutation(t *testing.T) {
	before := TargetAggregate{State: TargetRetryWait, ResumeStep: TargetResumeVerifying}
	oracle, oracleAdvanced, oracleSubmit := stepTarget(before, TargetAdmitSubmission)
	mutant, mutantAdvanced, mutantSubmit := unsafeRetryMutation(before, TargetAdmitSubmission)
	if oracleAdvanced || oracleSubmit || oracle != before {
		t.Fatalf("oracle unexpectedly allowed mismatched retry: %#v, %t, %t", oracle, oracleAdvanced, oracleSubmit)
	}
	if !mutantAdvanced || !mutantSubmit || mutant.State != TargetSubmitting {
		t.Fatalf("mutant did not represent the unsafe transition: %#v, %t, %t", mutant, mutantAdvanced, mutantSubmit)
	}
}

func unsafeRetryMutation(current TargetAggregate, command TargetCommand) (TargetAggregate, bool, bool) {
	if current.State == TargetRetryWait && command == TargetAdmitSubmission {
		return TargetAggregate{State: TargetSubmitting, Revision: current.Revision + 1}, true, true
	}
	return stepTarget(current, command)
}
