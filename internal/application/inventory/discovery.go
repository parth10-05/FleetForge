package inventory

import (
	"context"
	"fmt"
	"time"
)

// DiscoveryWorkType mirrors the persisted discovery vocabulary. Keeping the value
// here prevents a worker from silently claiming a broader due-work class.
const DiscoveryWorkType = "DISCOVER"

type DiscoveryWork struct {
	ID, TargetID, LeaseToken string
	ExpectedRevision         int64
	Attempt                  int
}

// DiscoveryTarget is the minimal trusted registration view a read-only
// executor needs. It contains an opaque credential reference, never secret
// material or a caller-controlled resource URI.
type DiscoveryTarget struct {
	Origin, TargetKey, CredentialReference string
}

type DiscoveryTargetLoader interface {
	LoadDiscoveryTarget(context.Context, DiscoveryWork) (DiscoveryTarget, error)
}
type DiscoveryClaimRequest struct {
	Owner, LeaseToken   string
	Now, LeaseExpiresAt time.Time
}
type DiscoveryResult struct {
	ObservationID string
	ObservedAt    time.Time
	Normalized    []byte
	Success       bool
}

type DiscoveryWorkClaimer interface {
	ClaimDiscoveryWork(context.Context, DiscoveryClaimRequest) (*DiscoveryWork, error)
}
type DiscoveryExecutor interface {
	Discover(context.Context, DiscoveryWork) (DiscoveryResult, error)
}
type DiscoveryResultWriter interface {
	RecordDiscoveryResult(context.Context, DiscoveryWork, DiscoveryResult) error
	RecordDiscoveryRetry(context.Context, DiscoveryWork, DiscoveryResult, time.Time, time.Time) error
	RetryDiscoveryWork(context.Context, DiscoveryWork, time.Time, time.Time) error
}

// DiscoveryRetrySchedule is evaluated only after a fenced work item has been
// claimed, so retry timing can depend on the durable attempt number.
type DiscoveryRetrySchedule func(DiscoveryWork, time.Time) time.Time

// RunDiscoveryOnce commits ownership before calling the discovery executor.
// It never exposes another work type or holds a database transaction during
// credential/transport activity.
func RunDiscoveryOnce(ctx context.Context, claimer DiscoveryWorkClaimer, executor DiscoveryExecutor, writer DiscoveryResultWriter, request DiscoveryClaimRequest, retrySchedule DiscoveryRetrySchedule) (bool, error) {
	if claimer == nil || executor == nil || writer == nil || retrySchedule == nil || request.Owner == "" || request.LeaseToken == "" || request.Now.IsZero() || !request.LeaseExpiresAt.After(request.Now) {
		return false, fmt.Errorf("invalid discovery worker dependencies")
	}
	work, err := claimer.ClaimDiscoveryWork(ctx, request)
	if err != nil {
		return false, fmt.Errorf("claim discovery work: %w", err)
	}
	if work == nil {
		return false, nil
	}
	if work.ID == "" || work.TargetID == "" || work.LeaseToken != request.LeaseToken || work.ExpectedRevision < 0 || work.Attempt < 1 {
		return true, fmt.Errorf("invalid claimed discovery work")
	}
	result, err := executor.Discover(ctx, *work)
	if err != nil {
		retryAt := retrySchedule(*work, request.Now)
		if retryAt.IsZero() || retryAt.Before(request.Now) {
			return true, fmt.Errorf("invalid discovery retry schedule")
		}
		if validDiscoveryResult(result) {
			return true, writer.RecordDiscoveryRetry(ctx, *work, result, request.Now, retryAt)
		}
		return true, writer.RetryDiscoveryWork(ctx, *work, request.Now, retryAt)
	}
	if !validDiscoveryResult(result) {
		return true, fmt.Errorf("invalid discovery result")
	}
	if err := writer.RecordDiscoveryResult(ctx, *work, result); err != nil {
		return true, fmt.Errorf("record discovery result: %w", err)
	}
	return true, nil
}

func validDiscoveryResult(result DiscoveryResult) bool {
	return result.ObservationID != "" && !result.ObservedAt.IsZero() && len(result.Normalized) != 0
}
