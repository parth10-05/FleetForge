package main

import (
	"context"
	"fmt"
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

func TestRetryAtUsesAttemptAwareDeterministicBoundedBackoff(t *testing.T) {
	now := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	first := retryAt(app.DiscoveryWork{ID: "work-1", Attempt: 1}, now)
	second := retryAt(app.DiscoveryWork{ID: "work-1", Attempt: 2}, now)
	secondAgain := retryAt(app.DiscoveryWork{ID: "work-1", Attempt: 2}, now)
	if !first.Equal(now.Add(time.Second)) {
		t.Fatalf("first retry=%s", first)
	}
	if second.Before(now.Add(2*time.Second)) || second.After(now.Add(2250*time.Millisecond)) {
		t.Fatalf("second retry=%s", second)
	}
	if !second.Equal(secondAgain) {
		t.Fatalf("retry jitter must be deterministic: %s != %s", second, secondAgain)
	}
	for i := 0; i < 1_000; i++ {
		retry := retryAt(app.DiscoveryWork{ID: fmt.Sprintf("work-%d", i), Attempt: 2}, now)
		if retry.Before(now.Add(2*time.Second)) || retry.After(now.Add(2250*time.Millisecond)) {
			t.Fatalf("work %d retry=%s exceeds jitter cap", i, retry)
		}
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
