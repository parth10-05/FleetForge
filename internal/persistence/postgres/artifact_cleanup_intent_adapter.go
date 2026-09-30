package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	app "github.com/parth10-05/fleetforge/internal/application/artifacts"
)

// ArtifactCleanupIntentAdapter exposes only the application cleanup ports.
// Storage remains outside this adapter and therefore outside its transactions.
type ArtifactCleanupIntentAdapter struct{ Pool *pgxpool.Pool }

func (adapter ArtifactCleanupIntentAdapter) ClaimArtifactCleanupIntent(ctx context.Context, owner, token string, now, expiresAt time.Time) (*app.CleanupIntent, error) {
	claim, err := ClaimArtifactCleanupIntent(ctx, adapter.Pool, owner, token, now, expiresAt)
	if err != nil || claim == nil {
		return nil, err
	}
	return &app.CleanupIntent{ArtifactID: claim.ArtifactID, GrantID: claim.GrantID, LeaseToken: claim.LeaseToken}, nil
}

func (adapter ArtifactCleanupIntentAdapter) CompleteArtifactCleanupIntent(ctx context.Context, claim app.CleanupIntent, now time.Time) error {
	if adapter.Pool == nil {
		return fmt.Errorf("artifact cleanup persistence is unavailable")
	}
	return CompleteArtifactCleanupIntent(ctx, adapter.Pool, CleanupIntentClaim{ArtifactID: claim.ArtifactID, GrantID: claim.GrantID, LeaseToken: claim.LeaseToken}, now)
}
