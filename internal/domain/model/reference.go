// Package model provides a deliberately independent, test-oriented reference
// model. It does not import production reducers or transition catalogs.
package model

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

// Step returns false for rejected and no-op commands. Its transition table is
// intentionally handwritten rather than derived from production structures.
func Step(current CampaignAggregate, command CampaignCommand) (CampaignAggregate, bool) {
	next := current
	advance := func(state CampaignState) (CampaignAggregate, bool) {
		next.State = state
		next.Revision++
		return next, true
	}
	switch current.State {
	case CampaignDraft:
		switch command {
		case CampaignBeginValidation:
			return advance(CampaignValidating)
		case CampaignCancel:
			return advance(CampaignCancelled)
		}
	case CampaignValidating:
		switch command {
		case CampaignValidationPassed:
			return advance(CampaignReady)
		case CampaignValidationFailed:
			return advance(CampaignDraft)
		}
	case CampaignReady:
		switch command {
		case CampaignStartCanary:
			return advance(CampaignRunningCanary)
		case CampaignCancel:
			return advance(CampaignCancelled)
		}
	case CampaignRunningCanary:
		switch command {
		case CampaignCanaryTerminal:
			return advance(CampaignEvaluatingCanary)
		case CampaignPause:
			return pause(next)
		case CampaignAbort:
			return abort(next)
		}
	case CampaignEvaluatingCanary:
		switch command {
		case CampaignApproveCanary:
			return advance(CampaignRunningWaves)
		case CampaignPause:
			return pause(next)
		case CampaignAbort:
			return abort(next)
		}
	case CampaignRunningWaves:
		switch command {
		case CampaignReleaseNextWave:
			return advance(CampaignRunningWaves)
		case CampaignComplete:
			return advance(CampaignCompleted)
		case CampaignPause:
			return pause(next)
		case CampaignAbort:
			return abort(next)
		}
	case CampaignPaused:
		switch command {
		case CampaignResume:
			if resumable(current.SuspendedPhase) {
				next.State, next.SuspendedPhase = current.SuspendedPhase, ""
				next.Revision++
				return next, true
			}
		case CampaignAbort:
			return abort(next)
		}
	case CampaignAborting:
		switch command {
		case CampaignAbortFailed:
			return advance(CampaignFailed)
		case CampaignAbortCancelled:
			return advance(CampaignCancelled)
		}
	}
	return current, false
}

func pause(next CampaignAggregate) (CampaignAggregate, bool) {
	next.SuspendedPhase = next.State
	next.State = CampaignPaused
	next.Revision++
	next.CommandEpoch++
	return next, true
}
func abort(next CampaignAggregate) (CampaignAggregate, bool) {
	next.State = CampaignAborting
	next.Revision++
	next.CommandEpoch++
	return next, true
}
func resumable(state CampaignState) bool {
	return state == CampaignRunningCanary || state == CampaignEvaluatingCanary || state == CampaignRunningWaves
}
