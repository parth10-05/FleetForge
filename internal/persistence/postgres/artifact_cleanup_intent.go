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

const maxArtifactCleanupRepairBatch = 25

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

// RepairExpiredArtifactCleanupIntents restores only abandoned cleanup leases
// to PENDING. It does not touch artifacts or objects; a later worker must
// claim the same exact grant-scoped intent before it can resolve a receipt.
func RepairExpiredArtifactCleanupIntents(ctx context.Context, pool *pgxpool.Pool, now time.Time, limit int) ([]string, error) {
	if pool == nil || now.IsZero() || limit <= 0 || limit > maxArtifactCleanupRepairBatch {
		return nil, fmt.Errorf("invalid cleanup intent repair request")
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return nil, fmt.Errorf("begin cleanup intent repair: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	rows, err := tx.Query(ctx, `SELECT artifact_id::text FROM artifact_cleanup_intents
WHERE state='CLAIMED' AND lease_expires_at <= $1
ORDER BY lease_expires_at,created_at,artifact_id LIMIT $2 FOR UPDATE SKIP LOCKED`, now.UTC(), limit)
	if err != nil {
		return nil, fmt.Errorf("lock expired cleanup intents: %w", err)
	}
	ids := make([]string, 0, limit)
	for rows.Next() {
		var artifactID string
		if err := rows.Scan(&artifactID); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan expired cleanup intent: %w", err)
		}
		ids = append(ids, artifactID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("read expired cleanup intents: %w", err)
	}
	rows.Close()
	for _, artifactID := range ids {
		tag, err := tx.Exec(ctx, `UPDATE artifact_cleanup_intents
SET state='PENDING',lease_token=NULL,lease_owner=NULL,lease_expires_at=NULL
WHERE artifact_id=$1::uuid AND state='CLAIMED' AND lease_expires_at <= $2`, artifactID, now.UTC())
		if err != nil {
			return nil, fmt.Errorf("repair expired cleanup intent: %w", err)
		}
		if tag.RowsAffected() != 1 {
			return nil, ErrLeaseConflict
		}
	}
	if err := tx.Commit(ctx); err != nil {
		if isSerialization(err) {
			return nil, fmt.Errorf("%w: cleanup intent repair", ErrLeaseConflict)
		}
		return nil, fmt.Errorf("commit cleanup intent repair: %w", err)
	}
	return ids, nil
}
