package execution

import (
	"context"
	"fmt"
)

// SubmitOnce is deliberately single-shot. Any retry decision belongs to a
// later durable reconciliation transaction; this coordinator never infers
// absence from a transport error and never calls the target twice.
func SubmitOnce(ctx context.Context, repository SubmitRepository, artifacts ExactArtifactReader, target SubmitTarget, work WorkFence, actionPath string) error {
	if repository == nil || artifacts == nil || target == nil || actionPath == "" || work.DueWorkID == "" || work.CommandID == "" || work.LeaseToken == "" || work.DeliveryGeneration < 0 || work.ExpectedRevision < 0 {
		return fmt.Errorf("invalid submit execution request")
	}
	attempt, err := repository.LoadAttempt(ctx, work)
	if err != nil {
		return fmt.Errorf("load immutable attempt: %w", err)
	}
	if attempt.AttemptID == "" || attempt.AdmittedRevision != work.ExpectedRevision || attempt.CommandEpoch < 0 {
		return fmt.Errorf("attempt does not match submit fence")
	}
	stream, err := OpenVerifiedExact(ctx, artifacts, attempt.Artifact)
	if err != nil {
		return fmt.Errorf("open exact artifact: %w", err)
	}
	defer stream.Close()
	result := target.Submit(ctx, SubmitRequest{Attempt: attempt, UpdateActionPath: actionPath}, stream)
	if result.FinishedAt.IsZero() {
		return fmt.Errorf("target returned result without finish time")
	}
	if !knownSubmitClass(result.Class) {
		return fmt.Errorf("target returned unknown submit class")
	}
	if err := repository.CommitSubmitResult(ctx, work, result); err != nil {
		return fmt.Errorf("commit submit result: %w", err)
	}
	return nil
}
func knownSubmitClass(class SubmitClass) bool {
	switch class {
	case SubmitAccepted, SubmitExplicitRejection, SubmitProvenPrewriteFailure, SubmitPossiblyAccepted, SubmitUnsafeResponse:
		return true
	default:
		return false
	}
}
