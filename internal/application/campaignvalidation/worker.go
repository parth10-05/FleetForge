package campaignvalidation

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const WorkType = "CAMPAIGN_VALIDATE"

type Work struct {
	ID, CommandID, CampaignID, LeaseToken string
	ExpectedRevision                      int64
	ExpectedCommandEpoch                  *int64
}
type ClaimRequest struct {
	Owner, LeaseToken   string
	Now, LeaseExpiresAt time.Time
}
type Claimer interface {
	ClaimCampaignValidationWork(context.Context, ClaimRequest) (*Work, error)
}
type Finisher interface {
	RetryCampaignValidationWork(context.Context, Work, time.Time, time.Time) error
}

// ValidationConflict means another valid owner superseded this worker. The
// claimed lease is left for the durable lease-repair path rather than retried
// by a stale worker.
type ValidationConflict struct{ Err error }

func (err ValidationConflict) Error() string { return err.Err.Error() }
func (err ValidationConflict) Unwrap() error { return err.Err }

// ValidationRetryable identifies the only processor failures the worker may
// reschedule directly. Correctable rejections are finalized by the processor.
type ValidationRetryable struct{ Err error }

func (err ValidationRetryable) Error() string { return err.Err.Error() }
func (err ValidationRetryable) Unwrap() error { return err.Err }

// ValidationRejected identifies a correctable validation failure. The
// processor must finalize its DRAFT outcome before returning success.
type ValidationRejected struct{ Err error }

func (err ValidationRejected) Error() string { return err.Err.Error() }
func (err ValidationRejected) Unwrap() error { return err.Err }

type Processor interface {
	ValidateCampaign(context.Context, Work) error
}
type Worker struct {
	claimer   Claimer
	finisher  Finisher
	processor Processor
	now       func() time.Time
}

func NewWorker(c Claimer, f Finisher, p Processor, n func() time.Time) *Worker {
	if n == nil {
		n = time.Now
	}
	return &Worker{c, f, p, n}
}
func (w *Worker) RunOnce(ctx context.Context, r ClaimRequest) (bool, error) {
	if w == nil || w.claimer == nil || w.finisher == nil || w.processor == nil || r.Owner == "" || r.LeaseToken == "" || r.Now.IsZero() || !r.LeaseExpiresAt.After(r.Now) {
		return false, fmt.Errorf("invalid campaign validation worker request")
	}
	work, err := w.claimer.ClaimCampaignValidationWork(ctx, r)
	if err != nil {
		return false, err
	}
	if work == nil {
		return false, nil
	}
	if work.ID == "" || work.CommandID == "" || work.CampaignID == "" || work.LeaseToken == "" || work.ExpectedRevision < 0 {
		return false, fmt.Errorf("invalid claimed campaign validation work")
	}
	now := w.now().UTC()
	if err := w.processor.ValidateCampaign(ctx, *work); err != nil {
		var retryable ValidationRetryable
		if errors.As(err, &retryable) {
			if e := w.finisher.RetryCampaignValidationWork(ctx, *work, now, now.Add(time.Second)); e != nil {
				return false, e
			}
			return true, nil
		}
		var conflict ValidationConflict
		if errors.As(err, &conflict) {
			return true, nil
		}
		return true, err
	}
	// A nil return means the processor committed either the success or
	// correctable-rejection finalizer, including exact work completion.
	return true, nil
}
