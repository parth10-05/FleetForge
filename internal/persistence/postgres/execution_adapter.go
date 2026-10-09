package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parth10-05/fleetforge/internal/application/execution"
)

// ExecutionAdapter provides the fenced persistence operations used by the
// P05 execution coordinator. Reads validate a currently claimed due-work row
// before any target call; result writes re-check the same fence atomically.
type ExecutionAdapter struct {
	Pool *pgxpool.Pool
	Now  func() time.Time
}

// ClaimSubmitWork claims the oldest eligible firmware submission only. A
// separate method avoids claiming arbitrary due work and deciding its type
// after a lease is already held.
func (adapter ExecutionAdapter) ClaimSubmitWork(ctx context.Context, owner, leaseToken string) (execution.WorkFence, bool, error) {
	if adapter.Pool == nil || owner == "" || leaseToken == "" {
		return execution.WorkFence{}, false, fmt.Errorf("invalid firmware submit claim")
	}
	now := time.Now().UTC()
	if adapter.Now != nil {
		now = adapter.Now().UTC()
	}
	tx, err := adapter.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return execution.WorkFence{}, false, fmt.Errorf("begin firmware submit claim: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var work execution.WorkFence
	err = tx.QueryRow(ctx, `SELECT id::text,command_id::text,delivery_generation,expected_revision,expected_command_epoch
FROM due_work WHERE work_type='SUBMIT' AND delivery_state='PENDING' AND not_before <= $1
ORDER BY not_before,created_at,id LIMIT 1 FOR UPDATE SKIP LOCKED`, now).Scan(&work.DueWorkID, &work.CommandID, &work.DeliveryGeneration, &work.ExpectedRevision, &work.ExpectedCommandEpoch)
	if err == pgx.ErrNoRows {
		if err := tx.Commit(ctx); err != nil {
			return execution.WorkFence{}, false, classifyDueWorkError(err, "commit empty firmware submit claim")
		}
		return execution.WorkFence{}, false, nil
	}
	if err != nil {
		return execution.WorkFence{}, false, classifyDueWorkError(err, "lock firmware submit work")
	}
	work.LeaseToken = leaseToken
	tag, err := tx.Exec(ctx, `UPDATE due_work SET delivery_state='CLAIMED',lease_token=$2::uuid,lease_owner=$3,lease_expires_at=$4,updated_at=$5
WHERE id=$1::uuid AND work_type='SUBMIT' AND delivery_state='PENDING'`, work.DueWorkID, work.LeaseToken, owner, now.Add(time.Minute), now)
	if err != nil {
		return execution.WorkFence{}, false, classifyDueWorkError(err, "claim firmware submit work")
	}
	if tag.RowsAffected() != 1 {
		return execution.WorkFence{}, false, fmt.Errorf("%w: firmware submit work changed", ErrLeaseConflict)
	}
	if err := tx.Commit(ctx); err != nil {
		return execution.WorkFence{}, false, classifyDueWorkError(err, "commit firmware submit claim")
	}
	return work, true, nil
}

func (adapter ExecutionAdapter) LoadAttempt(ctx context.Context, work execution.WorkFence) (execution.AttemptSnapshot, error) {
	if adapter.Pool == nil || work.DueWorkID == "" || work.CommandID == "" || work.LeaseToken == "" || work.ExpectedRevision < 0 || work.DeliveryGeneration < 0 {
		return execution.AttemptSnapshot{}, fmt.Errorf("invalid execution attempt fence")
	}
	var attempt execution.AttemptSnapshot
	err := adapter.Pool.QueryRow(ctx, `SELECT a.id::text,COALESCE(a.parent_attempt_id::text,''),a.submission_admission_id::text,a.target_id::text,a.target_update_id::text,a.campaign_id::text,
a.provider,a.container,a.object_key,a.object_version,a.object_size,a.sha256_digest,a.expected_version,'',a.request_fingerprint,COALESCE(a.remote_task_reference,''),a.captured_command_epoch,a.admitted_revision
FROM due_work d
JOIN submission_admissions s ON s.target_update_id=d.subject_id
JOIN firmware_update_attempts a ON a.id=s.active_attempt_id
WHERE d.id=$1::uuid AND d.command_id=$2::uuid AND d.delivery_generation=$3 AND d.delivery_state='CLAIMED'
AND d.lease_token=$4::uuid AND d.expected_revision=$5 AND d.expected_command_epoch IS NOT DISTINCT FROM $6
AND d.work_type IN ('SUBMIT','TASK_OBSERVE','INVENTORY_OBSERVE','HEALTH_OBSERVE')
AND ((d.work_type='SUBMIT' AND a.status='ADMITTED') OR (d.work_type<>'SUBMIT' AND a.status='ACCEPTED'))`, work.DueWorkID, work.CommandID, work.DeliveryGeneration, work.LeaseToken, work.ExpectedRevision, work.ExpectedCommandEpoch).Scan(
		&attempt.AttemptID, &attempt.ParentAttemptID, &attempt.AdmissionID, &attempt.TargetID, &attempt.TargetUpdateID, &attempt.CampaignID,
		&attempt.Artifact.Provider, &attempt.Artifact.Container, &attempt.Artifact.ObjectKey, &attempt.Artifact.ObjectVersion, &attempt.Artifact.Size, &attempt.Artifact.SHA256,
		&attempt.ExpectedVersion, &attempt.CredentialReference, &attempt.RequestFingerprint, &attempt.RemoteTaskReference, &attempt.CommandEpoch, &attempt.AdmittedRevision,
	)
	if err == pgx.ErrNoRows {
		return execution.AttemptSnapshot{}, fmt.Errorf("%w: no admitted attempt for claimed submit work", ErrLeaseConflict)
	}
	if err != nil {
		return execution.AttemptSnapshot{}, fmt.Errorf("load fenced firmware attempt: %w", err)
	}
	return attempt, nil
}

func (adapter ExecutionAdapter) CommitSubmitResult(ctx context.Context, work execution.WorkFence, result execution.SubmitResult) error {
	if adapter.Pool == nil {
		return fmt.Errorf("invalid execution adapter")
	}
	now := time.Now().UTC()
	if adapter.Now != nil {
		now = adapter.Now().UTC()
	}
	return CommitFirmwareSubmitResult(ctx, adapter.Pool, work, result, now)
}
