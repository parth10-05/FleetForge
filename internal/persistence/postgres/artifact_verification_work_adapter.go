package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	app "github.com/parth10-05/fleetforge/internal/application/artifacts"
)

// ArtifactVerificationWorkAdapter owns only the P02 verification work class;
// it cannot accidentally claim discovery or future firmware-effect work.
type ArtifactVerificationWorkAdapter struct{ Pool *pgxpool.Pool }

func (adapter ArtifactVerificationWorkAdapter) ClaimVerificationWork(ctx context.Context, request app.VerificationWorkClaimRequest) ([]app.VerificationWork, error) {
	if adapter.Pool == nil || request.Owner == "" || request.LeaseToken == "" || request.Now.IsZero() || !request.LeaseExpiresAt.After(request.Now) {
		return nil, fmt.Errorf("invalid artifact verification work claim")
	}
	tx, err := adapter.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return nil, fmt.Errorf("begin artifact verification claim: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var work app.VerificationWork
	err = tx.QueryRow(ctx, `SELECT id::text,subject_id::text,expected_revision,expected_command_epoch
FROM due_work
WHERE delivery_state='PENDING' AND not_before <= $1
  AND subject_type='ARTIFACT' AND work_type='VERIFY_ARTIFACT'
ORDER BY not_before,created_at,id LIMIT 1 FOR UPDATE SKIP LOCKED`, request.Now.UTC()).Scan(&work.ID, &work.ArtifactID, &work.ExpectedRevision, &work.ExpectedCommandEpoch)
	if err == pgx.ErrNoRows {
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit empty artifact verification claim: %w", err)
		}
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("lock artifact verification work: %w", err)
	}
	work.LeaseToken, work.WorkType = request.LeaseToken, app.VerificationWorkType
	tag, err := tx.Exec(ctx, `UPDATE due_work SET delivery_state='CLAIMED',lease_token=$2::uuid,lease_owner=$3,lease_expires_at=$4,updated_at=$5 WHERE id=$1::uuid AND delivery_state='PENDING'`, work.ID, work.LeaseToken, request.Owner, request.LeaseExpiresAt.UTC(), request.Now.UTC())
	if err != nil {
		return nil, fmt.Errorf("claim artifact verification work: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return nil, ErrLeaseConflict
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit artifact verification claim: %w", err)
	}
	return []app.VerificationWork{work}, nil
}

func (adapter ArtifactVerificationWorkAdapter) CompleteVerificationWork(ctx context.Context, work app.VerificationWork, now time.Time) error {
	if adapter.Pool == nil {
		return fmt.Errorf("artifact verification persistence is unavailable")
	}
	return CompleteDueWork(ctx, adapter.Pool, DueWorkResult{ID: work.ID, LeaseToken: work.LeaseToken, ExpectedRevision: work.ExpectedRevision, ExpectedCommandEpoch: work.ExpectedCommandEpoch, Now: now})
}

func (adapter ArtifactVerificationWorkAdapter) RetryVerificationWork(ctx context.Context, work app.VerificationWork, now, retryAt time.Time) error {
	if adapter.Pool == nil {
		return fmt.Errorf("artifact verification persistence is unavailable")
	}
	return FailDueWork(ctx, adapter.Pool, DueWorkResult{ID: work.ID, LeaseToken: work.LeaseToken, ExpectedRevision: work.ExpectedRevision, ExpectedCommandEpoch: work.ExpectedCommandEpoch, Now: now, RetryAt: retryAt})
}
