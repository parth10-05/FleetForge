package targetupdate

import (
	"github.com/parth10-05/fleetforge/internal/domain/policy"
	"github.com/parth10-05/fleetforge/internal/domain/value"
)

func authorizedTargetInput(id value.CommandID, command Command, revision value.Revision, epoch value.CommandEpoch) CommandInput {
	return CommandInput{
		ID: id, Command: command, ExpectedRevision: revision, ExpectedEpoch: epoch,
		Authorization:   policy.ApproveTargetTransition(),
		Submission:      policy.NewPersistedSubmissionAdmission(id, epoch),
		RetryAbsence:    policy.NewIndependentAbsenceAndLateCallerExclusion(),
		SuccessEvidence: policy.NewVersionAndHealthSuccess(),
	}
}
