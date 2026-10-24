package execution

import (
	"context"
	"fmt"
)

// PrecheckWorkClaimer grants a lease only for PRECHECK work. It cannot claim
// SUBMIT work, so this safe-read worker never obtains authority to send a
// firmware update.
type PrecheckWorkClaimer interface {
	ClaimPrecheckWork(context.Context, string, string) (WorkFence, bool, error)
}

// PrecheckCommandSource supplies the policy-approved immutable envelope and
// target credential reference for one claimed work item. It is intentionally
// separate from target observations: workers may not derive admissions,
// capacity limits, audit identities, or credentials from a device response.
type PrecheckCommandSource interface {
	LoadPrecheckCommand(context.Context, WorkFence) (AdmissionCommand, string, error)
}

// PrecheckWorker executes at most one bounded target precheck. The durable
// completion/admission transaction is owned by PrecheckAndAdmit; this worker
// never completes, requeues, or retries a claimed row itself.
type PrecheckWorker struct {
	Claimer    PrecheckWorkClaimer
	Repository PrecheckRepository
	Target     PrecheckTarget
	Commands   PrecheckCommandSource
}

func (worker PrecheckWorker) RunOnce(ctx context.Context, owner, leaseToken string) (bool, error) {
	if worker.Claimer == nil || worker.Repository == nil || worker.Target == nil || worker.Commands == nil || owner == "" || leaseToken == "" {
		return false, fmt.Errorf("invalid precheck worker")
	}
	work, claimed, err := worker.Claimer.ClaimPrecheckWork(ctx, owner, leaseToken)
	if err != nil || !claimed {
		return claimed, err
	}
	command, credentialReference, err := worker.Commands.LoadPrecheckCommand(ctx, work)
	if err != nil {
		return true, fmt.Errorf("load precheck command: %w", err)
	}
	if _, _, err := PrecheckAndAdmit(ctx, worker.Repository, worker.Target, work, command, credentialReference); err != nil {
		return true, err
	}
	return true, nil
}
