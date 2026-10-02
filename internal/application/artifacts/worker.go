package artifacts

import (
	"context"
	"fmt"
	"time"
)

const VerificationWorkType = "VERIFY_ARTIFACT"

type VerificationWork struct {
	ID, ArtifactID, LeaseToken, WorkType string
	ExpectedRevision                     int64
	ExpectedCommandEpoch                 *int64
}

type VerificationWorkClaimRequest struct {
	Owner, LeaseToken   string
	Now, LeaseExpiresAt time.Time
}

type VerificationWorkClaimer interface {
	ClaimVerificationWork(context.Context, VerificationWorkClaimRequest) ([]VerificationWork, error)
}

type VerificationWorkFinisher interface {
	CompleteVerificationWork(context.Context, VerificationWork, time.Time) error
	RetryVerificationWork(context.Context, VerificationWork, time.Time, time.Time) error
}

type Worker struct {
	service  *Service
	claimer  VerificationWorkClaimer
	finisher VerificationWorkFinisher
	now      func() time.Time
}

func NewWorker(service *Service, claimer VerificationWorkClaimer, finisher VerificationWorkFinisher, now func() time.Time) *Worker {
	if now == nil {
		now = time.Now
	}
	return &Worker{service: service, claimer: claimer, finisher: finisher, now: now}
}

// RunOnce owns one bounded verification lease. The claim adapter must filter
// to VERIFY_ARTIFACT; other durable work classes are never acknowledged here.
func (worker *Worker) RunOnce(ctx context.Context, request VerificationWorkClaimRequest) error {
	if worker == nil || worker.service == nil || worker.claimer == nil || worker.finisher == nil || request.Owner == "" || request.LeaseToken == "" || request.Now.IsZero() || !request.LeaseExpiresAt.After(request.Now) {
		return fmt.Errorf("invalid artifact verification worker request")
	}
	claimed, err := worker.claimer.ClaimVerificationWork(ctx, request)
	if err != nil {
		return fmt.Errorf("claim artifact verification work: %w", err)
	}
	for _, work := range claimed {
		if work.WorkType != VerificationWorkType || work.ID == "" || work.ArtifactID == "" || work.LeaseToken == "" || work.ExpectedRevision < 0 {
			return fmt.Errorf("invalid claimed artifact verification work")
		}
		now := worker.now().UTC()
		if err := worker.service.VerifyBoundArtifact(ctx, work.ArtifactID); err != nil {
			if retryErr := worker.finisher.RetryVerificationWork(ctx, work, now, now.Add(time.Second)); retryErr != nil {
				return fmt.Errorf("retry artifact verification work: %w", retryErr)
			}
			continue
		}
		if err := worker.finisher.CompleteVerificationWork(ctx, work, now); err != nil {
			return fmt.Errorf("complete artifact verification work: %w", err)
		}
	}
	return nil
}
