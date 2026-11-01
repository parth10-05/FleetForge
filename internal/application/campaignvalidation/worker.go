package campaignvalidation

import (
	"context"
	"fmt"
	"time"
)

const WorkType = "CAMPAIGN_VALIDATE"

type Work struct {
	ID, CampaignID, LeaseToken string
	ExpectedRevision           int64
}
type ClaimRequest struct {
	Owner, LeaseToken   string
	Now, LeaseExpiresAt time.Time
}
type Claimer interface {
	ClaimCampaignValidationWork(context.Context, ClaimRequest) (*Work, error)
}
type Finisher interface {
	CompleteCampaignValidationWork(context.Context, Work, time.Time) error
	RetryCampaignValidationWork(context.Context, Work, time.Time, time.Time) error
}
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
	if work.ID == "" || work.CampaignID == "" || work.LeaseToken == "" || work.ExpectedRevision < 0 {
		return false, fmt.Errorf("invalid claimed campaign validation work")
	}
	now := w.now().UTC()
	if err := w.processor.ValidateCampaign(ctx, *work); err != nil {
		if e := w.finisher.RetryCampaignValidationWork(ctx, *work, now, now.Add(time.Second)); e != nil {
			return false, e
		}
		return true, nil
	}
	return true, w.finisher.CompleteCampaignValidationWork(ctx, *work, now)
}
