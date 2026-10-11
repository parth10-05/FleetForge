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

// PrecheckStartRepository is intentionally smaller than the execution
// repository: consuming CANARY_START can only create bounded PRECHECK work.
type PrecheckStartRepository interface {
	StartFirmwarePrecheck(context.Context, WorkFence) error
}

// WakeupPrecheckStartHandler finalizes the fenced, side-effect-free
// CANARY_START transition. It has neither artifact access nor a target port.
type WakeupPrecheckStartHandler struct{ Repository PrecheckStartRepository }

func (handler WakeupPrecheckStartHandler) Supports(envelope orchestration.Envelope) bool {
	return envelope.QueueClass == orchestration.QueueUpdateStep && envelope.WorkType == "CANARY_START"
}

func (handler WakeupPrecheckStartHandler) HandleFinalized(ctx context.Context, work orchestration.ClaimedWork) error {
	if handler.Repository == nil || work.WorkType != "CANARY_START" {
		return fmt.Errorf("invalid precheck start wakeup")
	}
	return handler.Repository.StartFirmwarePrecheck(ctx, WorkFence{DueWorkID: work.DueWorkID, CommandID: work.CommandID, LeaseToken: work.LeaseToken, DeliveryGeneration: work.DeliveryGeneration, ExpectedRevision: work.ExpectedRevision, ExpectedCommandEpoch: work.ExpectedCommandEpoch})
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
