package targetupdate

import "errors"

var ErrUnknownResumeStep = errors.New("unknown target update resume step")

type ResumeStep string

const (
	ResumeStepPrecheck      ResumeStep = "PRECHECK"
	ResumeStepSubmitting    ResumeStep = "SUBMITTING"
	ResumeStepWaitingReboot ResumeStep = "WAITING_REBOOT"
	ResumeStepVerifying     ResumeStep = "VERIFYING"
)

var allResumeSteps = []ResumeStep{ResumeStepPrecheck, ResumeStepSubmitting, ResumeStepWaitingReboot, ResumeStepVerifying}

func ParseResumeStep(raw string) (ResumeStep, error) {
	for _, step := range allResumeSteps {
		if raw == string(step) {
			return step, nil
		}
	}
	return "", ErrUnknownResumeStep
}
func (step ResumeStep) String() string { return string(step) }
