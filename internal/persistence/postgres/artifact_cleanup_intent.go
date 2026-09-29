package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type CleanupIntentClaim struct {
	ArtifactID string
	GrantID    string
	LeaseToken string
}

// ClaimArtifactCleanupIntent obtains one bounded, fenced deletion authority.
// It does not resolve storage receipts or issue any external I/O.
func ClaimArtifactCleanupIntent(ctx context.Context, pool *pgxpool.Pool, owner, leaseToken string, now, expiresAt time.Time) (*CleanupIntentClaim, error) {
	if pool == nil || owner == "" || leaseToken == "" || now.IsZero() || !expiresAt.After(now) {
		return nil, fmt.Errorf("invalid cleanup intent claim")
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return nil, fmt.Errorf("begin cleanup intent claim: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	claim := &CleanupIntentClaim{LeaseToken: leaseToken}
	err = tx.QueryRow(ctx, `SELECT artifact_id::text,grant_id FROM artifact_cleanup_intents WHERE state='PENDING' ORDER BY created_at,artifact_id LIMIT 1 FOR UPDATE SKIP LOCKED`).Scan(&claim.ArtifactID, &claim.GrantID)
	if err == pgx.ErrNoRows {
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit empty cleanup intent claim: %w", err)
		}
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("lock cleanup intent: %w", err)
	}
	tag, err := tx.Exec(ctx, `UPDATE artifact_cleanup_intents SET state='CLAIMED',lease_token=$2::uuid,lease_owner=$3,lease_expires_at=$4 WHERE artifact_id=$1::uuid AND state='PENDING'`, claim.ArtifactID, leaseToken, owner, expiresAt.UTC())
	if err != nil {
		return nil, fmt.Errorf("claim cleanup intent: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return nil, ErrLeaseConflict
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit cleanup intent claim: %w", err)
	}
	return claim, nil
}

func CompleteArtifactCleanupIntent(ctx context.Context, pool *pgxpool.Pool, claim CleanupIntentClaim, now time.Time) error {
	if pool == nil || claim.ArtifactID == "" || claim.LeaseToken == "" || now.IsZero() {
		return fmt.Errorf("invalid cleanup intent completion")
	}
	tag, err := pool.Exec(ctx, `UPDATE artifact_cleanup_intents SET state='COMPLETED',completed_at=$3,lease_token=NULL,lease_owner=NULL,lease_expires_at=NULL WHERE artifact_id=$1::uuid AND state='CLAIMED' AND lease_token=$2::uuid`, claim.ArtifactID, claim.LeaseToken, now.UTC())
	if err != nil {
		return fmt.Errorf("complete cleanup intent: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrLeaseConflict
	}
	return nil
}
