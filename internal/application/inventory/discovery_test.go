package inventory

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRunDiscoveryOnceRetriesTransportFailureWithClaimedFence(t *testing.T) {
	now := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	work := DiscoveryWork{ID: "work-1", TargetID: "target-1", LeaseToken: "token-1", ExpectedRevision: 2}
	writer := &discoveryWriter{}
	processed, err := RunDiscoveryOnce(context.Background(), discoveryClaimer{work: &work}, discoveryExecutor{err: errors.New("timeout")}, writer, DiscoveryClaimRequest{Owner: "worker", LeaseToken: "token-1", Now: now, LeaseExpiresAt: now.Add(time.Minute)}, now.Add(time.Second))
	if err != nil || !processed || writer.retry.ID != work.ID || writer.retry.LeaseToken != work.LeaseToken || !writer.retryAt.Equal(now.Add(time.Second)) {
		t.Fatalf("processed=%v err=%v retry=%+v retryAt=%s", processed, err, writer.retry, writer.retryAt)
	}
}

func TestRunDiscoveryOncePersistsRetryableFailureBeforeReleasingLease(t *testing.T) {
	now := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	work := DiscoveryWork{ID: "work-1", TargetID: "target-1", LeaseToken: "token-1", ExpectedRevision: 2}
	writer := &discoveryWriter{}
	result := DiscoveryResult{ObservationID: "observation-1", ObservedAt: now, Normalized: []byte(`{"error_kind":"timeout"}`)}
	processed, err := RunDiscoveryOnce(context.Background(), discoveryClaimer{work: &work}, discoveryExecutor{result: result, err: errors.New("timeout")}, writer, DiscoveryClaimRequest{Owner: "worker", LeaseToken: "token-1", Now: now, LeaseExpiresAt: now.Add(time.Minute)}, now.Add(time.Second))
	if err != nil || !processed || writer.retryResult.ObservationID != result.ObservationID || writer.retry.ID != work.ID || !writer.retryAt.Equal(now.Add(time.Second)) {
		t.Fatalf("processed=%v err=%v result=%+v retry=%+v retryAt=%s", processed, err, writer.retryResult, writer.retry, writer.retryAt)
	}
}

type discoveryClaimer struct{ work *DiscoveryWork }

func (c discoveryClaimer) ClaimDiscoveryWork(context.Context, DiscoveryClaimRequest) (*DiscoveryWork, error) {
	return c.work, nil
}

type discoveryExecutor struct {
	result DiscoveryResult
	err    error
}

func (e discoveryExecutor) Discover(context.Context, DiscoveryWork) (DiscoveryResult, error) {
	return e.result, e.err
}

type discoveryWriter struct {
	retry       DiscoveryWork
	retryAt     time.Time
	retryResult DiscoveryResult
}

func (w *discoveryWriter) RecordDiscoveryResult(context.Context, DiscoveryWork, DiscoveryResult) error {
	return nil
}
func (w *discoveryWriter) RecordDiscoveryRetry(_ context.Context, work DiscoveryWork, result DiscoveryResult, _ time.Time, retryAt time.Time) error {
	w.retry, w.retryResult, w.retryAt = work, result, retryAt
	return nil
}
func (w *discoveryWriter) RetryDiscoveryWork(_ context.Context, work DiscoveryWork, _ time.Time, retryAt time.Time) error {
	w.retry, w.retryAt = work, retryAt
	return nil
}
