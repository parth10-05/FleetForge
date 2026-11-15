package campaignvalidation

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestWorkerCompletesOrRetriesClaimedWork(t *testing.T) {
	now := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name  string
		err   error
		retry int
	}{{"finalized", nil, 0}, {"retry", ValidationRetryable{Err: errors.New("temporary")}, 1}, {"conflict", ValidationConflict{Err: errors.New("stale")}, 0}} {
		t.Run(test.name, func(t *testing.T) {
			finisher := &workerFinisher{}
			worker := NewWorker(workerClaimer{work: &Work{ID: "work", CommandID: "command", CampaignID: "campaign", LeaseToken: "lease", ExpectedRevision: 1}}, finisher, workerProcessor{err: test.err}, func() time.Time { return now })
			ran, err := worker.RunOnce(context.Background(), ClaimRequest{Owner: "owner", LeaseToken: "token", Now: now, LeaseExpiresAt: now.Add(time.Minute)})
			if err != nil || !ran || finisher.retry != test.retry {
				t.Fatalf("ran=%v err=%v retry=%d", ran, err, finisher.retry)
			}
		})
	}
}

type workerClaimer struct{ work *Work }

func (c workerClaimer) ClaimCampaignValidationWork(context.Context, ClaimRequest) (*Work, error) {
	return c.work, nil
}

type workerProcessor struct{ err error }

func (p workerProcessor) ValidateCampaign(context.Context, Work) error { return p.err }

type workerFinisher struct{ retry int }

func (f *workerFinisher) RetryCampaignValidationWork(context.Context, Work, time.Time, time.Time) error {
	f.retry++
	return nil
}
