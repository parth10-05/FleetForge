package artifacts

import (
	"context"
	"fmt"
	"time"

	"github.com/parth10-05/fleetforge/internal/domain/artifact"
)

// CleanupIntent is the application-level view of one durable, lease-fenced
// exact-delete authorization.
type CleanupIntent struct {
	ArtifactID string
	GrantID    string
	LeaseToken string
}

type CleanupIntentClaimer interface {
	ClaimArtifactCleanupIntent(context.Context, string, string, time.Time, time.Time) (*CleanupIntent, error)
}

type CleanupIntentCompleter interface {
	CompleteArtifactCleanupIntent(context.Context, CleanupIntent, time.Time) error
}

type UploadReceiptResolver interface {
	Receipt(string) (artifact.ObjectIdentity, error)
}

type ExactObjectDeleter interface {
	DeleteExact(artifact.ObjectIdentity) error
}

// RunCleanupOnce contains no database transaction around storage I/O. A
// durable claim commits first; only its grant resolves to the immutable tuple
// passed to DeleteExact; completion is fenced by the original token.
func RunCleanupOnce(ctx context.Context, claimer CleanupIntentClaimer, completer CleanupIntentCompleter, receipts UploadReceiptResolver, deleter ExactObjectDeleter, owner, token string, now, expiresAt time.Time) (bool, error) {
	if claimer == nil || completer == nil || receipts == nil || deleter == nil || owner == "" || token == "" || now.IsZero() || !expiresAt.After(now) {
		return false, fmt.Errorf("invalid cleanup dependencies")
	}
	claim, err := claimer.ClaimArtifactCleanupIntent(ctx, owner, token, now, expiresAt)
	if err != nil {
		return false, fmt.Errorf("claim cleanup intent: %w", err)
	}
	if claim == nil {
		return false, nil
	}
	identity, err := receipts.Receipt(claim.GrantID)
	if err != nil {
		return true, fmt.Errorf("resolve cleanup receipt: %w", err)
	}
	if err := deleter.DeleteExact(identity); err != nil {
		return true, fmt.Errorf("delete exact cleanup object: %w", err)
	}
	if err := completer.CompleteArtifactCleanupIntent(ctx, *claim, now); err != nil {
		return true, fmt.Errorf("complete cleanup intent: %w", err)
	}
	return true, nil
}
