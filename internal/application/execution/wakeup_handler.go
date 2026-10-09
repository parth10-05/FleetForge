package execution

import (
	"context"
	"fmt"

	"github.com/parth10-05/fleetforge/internal/application/orchestration"
)

// WakeupSubmitHandler binds a wakeup lease to exactly one SUBMIT transition.
// Its durable result transaction completes the due work, so the orchestration
// worker must dispatch it through FinalizingWorkHandler rather than calling
// its generic completion path afterwards.
type WakeupSubmitHandler struct {
	Repository SubmitRepository
	Artifacts  ExactArtifactReader
	Target     SubmitTarget
	ActionPath string
}

func (handler WakeupSubmitHandler) Supports(envelope orchestration.Envelope) bool {
	return envelope.QueueClass == orchestration.QueueUpdateStep && envelope.WorkType == "SUBMIT"
}

func (handler WakeupSubmitHandler) HandleFinalized(ctx context.Context, work orchestration.ClaimedWork) error {
	if handler.Repository == nil || handler.Artifacts == nil || handler.Target == nil || handler.ActionPath == "" {
		return fmt.Errorf("invalid wakeup submit handler")
	}
	if work.WorkType != "SUBMIT" {
		return fmt.Errorf("wakeup submit handler received non-submit work")
	}
	return SubmitOnce(ctx, handler.Repository, handler.Artifacts, handler.Target, WorkFence{
		DueWorkID:            work.DueWorkID,
		CommandID:            work.CommandID,
		LeaseToken:           work.LeaseToken,
		DeliveryGeneration:   work.DeliveryGeneration,
		ExpectedRevision:     work.ExpectedRevision,
		ExpectedCommandEpoch: work.ExpectedCommandEpoch,
	}, handler.ActionPath)
}
