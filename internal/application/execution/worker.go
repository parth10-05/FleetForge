package execution

import (
	"context"
	"fmt"
)

// SubmitWorkClaimer returns only an exact claimed SUBMIT work fence. It never
// returns a generic due-work row, preventing this worker from accidentally
// consuming an unrelated operation's wakeup.
type SubmitWorkClaimer interface {
	ClaimSubmitWork(context.Context, string, string) (WorkFence, bool, error)
}

type SubmitWorker struct {
	Claimer    SubmitWorkClaimer
	Repository SubmitRepository
	Artifacts  ExactArtifactReader
	Target     SubmitTarget
	ActionPath string
}

// RunOnce claims at most one target-side submission and delegates its exact
// result to SubmitOnce. SubmitOnce durably completes the work itself; this
// worker must never complete or requeue a side-effecting work row separately.
func (worker SubmitWorker) RunOnce(ctx context.Context, owner, leaseToken string) (bool, error) {
	if worker.Claimer == nil || worker.Repository == nil || worker.Artifacts == nil || worker.Target == nil || worker.ActionPath == "" || owner == "" || leaseToken == "" {
		return false, fmt.Errorf("invalid submit worker")
	}
	work, claimed, err := worker.Claimer.ClaimSubmitWork(ctx, owner, leaseToken)
	if err != nil || !claimed {
		return claimed, err
	}
	if err := SubmitOnce(ctx, worker.Repository, worker.Artifacts, worker.Target, work, worker.ActionPath); err != nil {
		return true, err
	}
	return true, nil
}
