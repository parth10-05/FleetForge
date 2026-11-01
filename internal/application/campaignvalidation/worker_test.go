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
		name            string
		err             error
		complete, retry int
	}{{"complete", nil, 1, 0}, {"retry", errors.New("blocked"), 0, 1}} {
		t.Run(test.name, func(t *testing.T) {
			finisher := &workerFinisher{}
			worker := NewWorker(workerClaimer{work: &Work{ID: "work", CampaignID: "campaign", LeaseToken: "lease", ExpectedRevision: 1}}, finisher, workerProcessor{err: test.err}, func() time.Time { return now })
			ran, err := worker.RunOnce(context.Background(), ClaimRequest{Owner: "owner", LeaseToken: "token", Now: now, LeaseExpiresAt: now.Add(time.Minute)})
			if err != nil || !ran || finisher.complete != test.complete || finisher.retry != test.retry {
				t.Fatalf("ran=%v err=%v complete=%d retry=%d", ran, err, finisher.complete, finisher.retry)
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

type workerFinisher struct{ complete, retry int }

func (f *workerFinisher) CompleteCampaignValidationWork(context.Context, Work, time.Time) error {
	f.complete++
	return nil
}
func (f *workerFinisher) RetryCampaignValidationWork(context.Context, Work, time.Time, time.Time) error {
	f.retry++
	return nil
}
