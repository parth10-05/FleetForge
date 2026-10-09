package execution

import (
	"context"
	"fmt"
)

// SubmitOnce is deliberately single-shot. Any retry decision belongs to a
// later durable reconciliation transaction; this coordinator never infers
// absence from a transport error and never calls the target twice.
func SubmitOnce(ctx context.Context, repository ExecutionRepository, artifacts ExactArtifactReader, target RedfishExecutionPort, work WorkFence, attemptID, actionPath string) error {
	if repository == nil || artifacts == nil || target == nil || attemptID == "" || actionPath == "" || work.DueWorkID == "" || work.CommandID == "" || work.LeaseToken == "" || work.DeliveryGeneration < 0 || work.ExpectedRevision < 0 {
		return fmt.Errorf("invalid submit execution request")
	}
	attempt, err := repository.LoadAttempt(ctx, attemptID)
	if err != nil {
		return fmt.Errorf("load immutable attempt: %w", err)
	}
	if attempt.AttemptID != attemptID {
		return fmt.Errorf("attempt identity mismatch")
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
