package main

import (
	"context"
	"testing"
	"time"

	app "github.com/parth10-05/fleetforge/internal/application/inventory"
)

func TestRunOnceUsesBoundedDiscoveryLease(t *testing.T) {
	claimer := &claimRecorder{}
	err := runOnce(context.Background(), claimer, noWorkExecutor{}, noWorkWriter{}, "worker")
	if err != nil || claimer.request.Owner != "worker" || claimer.request.LeaseExpiresAt.Sub(claimer.request.Now) != 30*time.Second {
		t.Fatalf("err=%v claim=%+v", err, claimer.request)
	}
}

type claimRecorder struct{ request app.DiscoveryClaimRequest }

func (c *claimRecorder) ClaimDiscoveryWork(_ context.Context, request app.DiscoveryClaimRequest) (*app.DiscoveryWork, error) {
	c.request = request
	return nil, nil
}

type noWorkExecutor struct{}

func (noWorkExecutor) Discover(context.Context, app.DiscoveryWork) (app.DiscoveryResult, error) {
	return app.DiscoveryResult{}, nil
}

type noWorkWriter struct{}

func (noWorkWriter) RecordDiscoveryResult(context.Context, app.DiscoveryWork, app.DiscoveryResult) error {
	return nil
}
func (noWorkWriter) RecordDiscoveryRetry(context.Context, app.DiscoveryWork, app.DiscoveryResult, time.Time, time.Time) error {
	return nil
}
func (noWorkWriter) RetryDiscoveryWork(context.Context, app.DiscoveryWork, time.Time, time.Time) error {
	return nil
}
