package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	app "github.com/parth10-05/fleetforge/internal/application/inventory"
)

// DiscoveryWorkAdapter owns only P02's read-only discovery work. In
// particular, it cannot claim firmware submission or any future work class.
// The final result transaction deliberately updates the observation, head, and
// leased work row together, so a crash cannot leave a committed observation
// with a live work lease (or vice versa).
type DiscoveryWorkAdapter struct{ Pool *pgxpool.Pool }

const maxDiscoveryAttempts = 3

// LoadDiscoveryTarget reads one complete, active registration without taking a
// transaction lock. The executor uses the resulting value only after this
// query has returned, so network activity cannot hold workflow locks.
func (adapter DiscoveryWorkAdapter) LoadDiscoveryTarget(ctx context.Context, work app.DiscoveryWork) (app.DiscoveryTarget, error) {
	if adapter.Pool == nil || work.TargetID == "" || work.ExpectedRevision < 0 {
		return app.DiscoveryTarget{}, fmt.Errorf("invalid discovery target request")
	}
	var target app.DiscoveryTarget
	err := adapter.Pool.QueryRow(ctx, `SELECT endpoint_reference,target_key,credential_reference
FROM targets WHERE id=$1::uuid AND revision=$2 AND administrative_state='ACTIVE'
  AND trust_metadata_complete AND contract_version='synthetic-redfish-v1'
  AND credential_provider='synthetic-test' AND transport_policy_version='synthetic-redfish-v1'`, work.TargetID, work.ExpectedRevision).Scan(&target.Origin, &target.TargetKey, &target.CredentialReference)
	if err != nil {
		if err == pgx.ErrNoRows {
			return app.DiscoveryTarget{}, fmt.Errorf("discovery target is unavailable")
		}
		return app.DiscoveryTarget{}, fmt.Errorf("load discovery target: %w", err)
	}
	return target, nil
}

func (adapter DiscoveryWorkAdapter) ClaimDiscoveryWork(ctx context.Context, request app.DiscoveryClaimRequest) (*app.DiscoveryWork, error) {
	if adapter.Pool == nil || request.Owner == "" || request.LeaseToken == "" || request.Now.IsZero() || !request.LeaseExpiresAt.After(request.Now) {
		return nil, fmt.Errorf("invalid discovery work claim")
	}
	tx, err := adapter.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return nil, fmt.Errorf("begin discovery claim: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	work := &app.DiscoveryWork{LeaseToken: request.LeaseToken}
	err = tx.QueryRow(ctx, `SELECT id::text, subject_id::text, expected_revision
FROM due_work
WHERE delivery_state='PENDING' AND not_before <= $1
  AND subject_type='TARGET' AND work_type='DISCOVER'
  AND discovery_attempts < $2
ORDER BY not_before, created_at, id LIMIT 1 FOR UPDATE SKIP LOCKED`, request.Now.UTC(), maxDiscoveryAttempts).Scan(&work.ID, &work.TargetID, &work.ExpectedRevision)
	if err == pgx.ErrNoRows {
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit empty discovery claim: %w", err)
		}
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("lock discovery work: %w", err)
	}
	tag, err := tx.Exec(ctx, `UPDATE due_work
SET delivery_state='CLAIMED', discovery_attempts=discovery_attempts+1,
    lease_token=$2::uuid, lease_owner=$3,
    lease_expires_at=$4, updated_at=$5
WHERE id=$1::uuid AND delivery_state='PENDING'`, work.ID, request.LeaseToken, request.Owner, request.LeaseExpiresAt.UTC(), request.Now.UTC())
	if err != nil {
		return nil, fmt.Errorf("claim discovery work: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return nil, ErrLeaseConflict
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit discovery claim: %w", err)
	}
	return work, nil
}

func (adapter DiscoveryWorkAdapter) RecordDiscoveryResult(ctx context.Context, work app.DiscoveryWork, result app.DiscoveryResult) error {
	return adapter.persistDiscoveryResult(ctx, work, result, result.ObservedAt, nil)
}

// RecordDiscoveryRetry retains a normalized retryable failure and releases the
// exact lease in one transaction. A crash therefore cannot leave the latest
// failure evidence committed while the old lease remains live.
func (adapter DiscoveryWorkAdapter) RecordDiscoveryRetry(ctx context.Context, work app.DiscoveryWork, result app.DiscoveryResult, now, retryAt time.Time) error {
	if now.IsZero() || retryAt.IsZero() || retryAt.Before(now) {
		return fmt.Errorf("invalid discovery retry time")
	}
	return adapter.persistDiscoveryResult(ctx, work, result, now, &retryAt)
}

func (adapter DiscoveryWorkAdapter) persistDiscoveryResult(ctx context.Context, work app.DiscoveryWork, result app.DiscoveryResult, finishedAt time.Time, retryAt *time.Time) error {
	if adapter.Pool == nil || work.ID == "" || work.TargetID == "" || work.LeaseToken == "" || work.ExpectedRevision < 0 || result.ObservationID == "" || result.ObservedAt.IsZero() || finishedAt.IsZero() || len(result.Normalized) == 0 || len(result.Normalized) > 32768 || !json.Valid(result.Normalized) {
		return fmt.Errorf("invalid discovery result persistence request")
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(result.Normalized, &object); err != nil || object == nil {
		return fmt.Errorf("discovery result must be a JSON object")
	}
	tx, err := adapter.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("begin discovery result: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Target precedes due work in this transaction's lock order. External I/O
	// already completed before this point, so these locks cover DB state only.
	var revision int64
	if err := tx.QueryRow(ctx, "SELECT revision FROM targets WHERE id=$1::uuid FOR UPDATE", work.TargetID).Scan(&revision); err != nil {
		return fmt.Errorf("lock discovery target: %w", err)
	}
	if revision != work.ExpectedRevision {
		return fmt.Errorf("%w: discovery target revision changed", ErrRevisionConflict)
	}
	var persistedWorkID string
	var attempts int
	if err := tx.QueryRow(ctx, `SELECT id::text,discovery_attempts FROM due_work
WHERE id=$1::uuid AND subject_type='TARGET' AND subject_id=$2::uuid
  AND work_type='DISCOVER' AND delivery_state='CLAIMED'
  AND lease_token=$3::uuid AND expected_revision=$4
  AND expected_command_epoch IS NULL FOR UPDATE`, work.ID, work.TargetID, work.LeaseToken, work.ExpectedRevision).Scan(&persistedWorkID, &attempts); err != nil {
		if err == pgx.ErrNoRows {
			return ErrLeaseConflict
		}
		return fmt.Errorf("lock claimed discovery work: %w", err)
	}
	kind := "DISCOVERY_FAILURE"
	if result.Success {
		kind = "INVENTORY"
	}
	if _, err := tx.Exec(ctx, `INSERT INTO device_observations
    (id,target_id,kind,observed_at,normalized_data)
VALUES ($1::uuid,$2::uuid,$3,$4,$5)`, result.ObservationID, work.TargetID, kind, result.ObservedAt.UTC(), result.Normalized); err != nil {
		return fmt.Errorf("insert discovery observation: %w", err)
	}
	if result.Success {
		_, err = tx.Exec(ctx, `INSERT INTO target_discovery_heads
    (target_id,head_revision,configuration_revision,latest_attempt_id,last_good_id)
VALUES ($1::uuid,1,$2,$3::uuid,$3::uuid)
ON CONFLICT (target_id) DO UPDATE SET
    head_revision=target_discovery_heads.head_revision+1,
    configuration_revision=EXCLUDED.configuration_revision,
    latest_attempt_id=EXCLUDED.latest_attempt_id,
    last_good_id=EXCLUDED.last_good_id`, work.TargetID, work.ExpectedRevision, result.ObservationID)
	} else {
		_, err = tx.Exec(ctx, `INSERT INTO target_discovery_heads
    (target_id,head_revision,configuration_revision,latest_attempt_id)
VALUES ($1::uuid,1,$2,$3::uuid)
ON CONFLICT (target_id) DO UPDATE SET
    head_revision=target_discovery_heads.head_revision+1,
    configuration_revision=EXCLUDED.configuration_revision,
    latest_attempt_id=EXCLUDED.latest_attempt_id`, work.TargetID, work.ExpectedRevision, result.ObservationID)
	}
	if err != nil {
		return fmt.Errorf("advance discovery head: %w", err)
	}
	var tag pgconnCommandTag
	if retryAt == nil || attempts >= maxDiscoveryAttempts {
		tag, err = tx.Exec(ctx, `UPDATE due_work SET delivery_state='COMPLETED',
    lease_token=NULL, lease_owner=NULL, lease_expires_at=NULL, updated_at=$5
WHERE id=$1::uuid AND subject_type='TARGET' AND subject_id=$2::uuid
  AND work_type='DISCOVER' AND delivery_state='CLAIMED' AND lease_token=$3::uuid
  AND expected_revision=$4 AND expected_command_epoch IS NULL`, work.ID, work.TargetID, work.LeaseToken, work.ExpectedRevision, finishedAt.UTC())
	} else {
		tag, err = tx.Exec(ctx, `UPDATE due_work SET delivery_state='PENDING', not_before=$5,
    lease_token=NULL, lease_owner=NULL, lease_expires_at=NULL, updated_at=$6
WHERE id=$1::uuid AND subject_type='TARGET' AND subject_id=$2::uuid
  AND work_type='DISCOVER' AND delivery_state='CLAIMED' AND lease_token=$3::uuid
  AND expected_revision=$4 AND expected_command_epoch IS NULL`, work.ID, work.TargetID, work.LeaseToken, work.ExpectedRevision, retryAt.UTC(), finishedAt.UTC())
	}
	if err != nil {
		return fmt.Errorf("complete discovery work: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrLeaseConflict
	}
	if err := tx.Commit(ctx); err != nil {
		if isSerialization(err) {
			return fmt.Errorf("%w: discovery result commit", ErrLeaseConflict)
		}
		return fmt.Errorf("commit discovery result: %w", err)
	}
	return nil
}

func (adapter DiscoveryWorkAdapter) RetryDiscoveryWork(ctx context.Context, work app.DiscoveryWork, now, retryAt time.Time) error {
	if adapter.Pool == nil || work.ID == "" || work.TargetID == "" || work.LeaseToken == "" || work.ExpectedRevision < 0 || now.IsZero() || retryAt.IsZero() || retryAt.Before(now) {
		return fmt.Errorf("discovery persistence is unavailable")
	}
	tag, err := adapter.Pool.Exec(ctx, `UPDATE due_work SET
delivery_state=CASE WHEN discovery_attempts >= $5 THEN 'COMPLETED' ELSE 'PENDING' END,
not_before=CASE WHEN discovery_attempts >= $5 THEN not_before ELSE $6 END,
lease_token=NULL,lease_owner=NULL,lease_expires_at=NULL,updated_at=$7
WHERE id=$1::uuid AND subject_type='TARGET' AND subject_id=$2::uuid
  AND work_type='DISCOVER' AND delivery_state='CLAIMED' AND lease_token=$3::uuid
  AND expected_revision=$4 AND expected_command_epoch IS NULL`, work.ID, work.TargetID, work.LeaseToken, work.ExpectedRevision, maxDiscoveryAttempts, retryAt.UTC(), now.UTC())
	if err != nil {
		return fmt.Errorf("retry discovery work: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrLeaseConflict
	}
	return nil
}
