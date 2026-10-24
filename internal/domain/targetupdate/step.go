package targetupdate

import "errors"

var ErrUnknownResumeStep = errors.New("unknown target update resume step")

type ResumeStep string

const (
	ResumeStepPrecheck           ResumeStep = "PRECHECK"
	ResumeStepSubmitAdmission    ResumeStep = "SUBMIT_ADMISSION"
	ResumeStepTaskObserve        ResumeStep = "TASK_OBSERVE"
	ResumeStepRebootWait         ResumeStep = "REBOOT_WAIT"
	ResumeStepVerifyVersion      ResumeStep = "VERIFY_VERSION"
	ResumeStepVerifyHealth       ResumeStep = "VERIFY_HEALTH"
	ResumeStepAmbiguityReconcile ResumeStep = "AMBIGUITY_RECONCILE"
)

// Resume steps are exact execution boundaries. In particular, no observation
// or verification retry can be represented as a submission retry.
var allResumeSteps = []ResumeStep{ResumeStepPrecheck, ResumeStepSubmitAdmission, ResumeStepTaskObserve, ResumeStepRebootWait, ResumeStepVerifyVersion, ResumeStepVerifyHealth, ResumeStepAmbiguityReconcile}

func ParseResumeStep(raw string) (ResumeStep, error) {
	for _, step := range allResumeSteps {
		if raw == string(step) {
			return step, nil
		}
	}
	return "", ErrUnknownResumeStep
}
func (step ResumeStep) String() string { return string(step) }
