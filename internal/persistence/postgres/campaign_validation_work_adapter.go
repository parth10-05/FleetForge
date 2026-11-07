package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	app "github.com/parth10-05/fleetforge/internal/application/campaignvalidation"
)

// CampaignValidationWorkAdapter owns only campaign-validation leases.
type CampaignValidationWorkAdapter struct{ Pool *pgxpool.Pool }

func (adapter CampaignValidationWorkAdapter) ClaimCampaignValidationWork(ctx context.Context, request app.ClaimRequest) (*app.Work, error) {
	if adapter.Pool == nil || request.Owner == "" || request.LeaseToken == "" || request.Now.IsZero() || !request.LeaseExpiresAt.After(request.Now) {
		return nil, fmt.Errorf("invalid campaign validation work claim")
	}
	tx, err := adapter.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return nil, fmt.Errorf("begin campaign validation claim: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	work := &app.Work{}
	err = tx.QueryRow(ctx, `SELECT id::text,command_id::text,subject_id::text,expected_revision,expected_command_epoch FROM due_work WHERE delivery_state='PENDING' AND not_before <= $1 AND subject_type='CAMPAIGN' AND work_type='CAMPAIGN_VALIDATE' ORDER BY not_before,created_at,id LIMIT 1 FOR UPDATE SKIP LOCKED`, request.Now.UTC()).Scan(&work.ID, &work.CommandID, &work.CampaignID, &work.ExpectedRevision, &work.ExpectedCommandEpoch)
	if err == pgx.ErrNoRows {
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit empty campaign validation claim: %w", err)
		}
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("lock campaign validation work: %w", err)
	}
	work.LeaseToken = request.LeaseToken
	tag, err := tx.Exec(ctx, `UPDATE due_work SET delivery_state='CLAIMED',lease_token=$2::uuid,lease_owner=$3,lease_expires_at=$4,updated_at=$5 WHERE id=$1::uuid AND delivery_state='PENDING'`, work.ID, work.LeaseToken, request.Owner, request.LeaseExpiresAt.UTC(), request.Now.UTC())
	if err != nil {
		return nil, fmt.Errorf("claim campaign validation work: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return nil, ErrLeaseConflict
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit campaign validation claim: %w", err)
	}
	return work, nil
}
func (adapter CampaignValidationWorkAdapter) CompleteCampaignValidationWork(ctx context.Context, work app.Work, now time.Time) error {
	return CompleteDueWork(ctx, adapter.Pool, DueWorkResult{ID: work.ID, LeaseToken: work.LeaseToken, ExpectedRevision: work.ExpectedRevision, ExpectedCommandEpoch: work.ExpectedCommandEpoch, Now: now})
}
func (adapter CampaignValidationWorkAdapter) RetryCampaignValidationWork(ctx context.Context, work app.Work, now, retryAt time.Time) error {
	return FailDueWork(ctx, adapter.Pool, DueWorkResult{ID: work.ID, LeaseToken: work.LeaseToken, ExpectedRevision: work.ExpectedRevision, ExpectedCommandEpoch: work.ExpectedCommandEpoch, Now: now, RetryAt: retryAt})
}

var _ app.Claimer = CampaignValidationWorkAdapter{}
var _ app.Finisher = CampaignValidationWorkAdapter{}
