package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parth10-05/fleetforge/internal/application/execution"
)

// CommitFirmwareSubmitResult atomically records a single target invocation,
// advances the fenced target update, and completes the claimed submit work.
// It intentionally has no retry branch: all uncertain outcomes become durable
// reconciliation work and retain the physical reservation.
func CommitFirmwareSubmitResult(ctx context.Context, pool *pgxpool.Pool, work execution.WorkFence, result execution.SubmitResult, now time.Time) error {
	if pool == nil || now.IsZero() || work.DueWorkID == "" || work.CommandID == "" || work.LeaseToken == "" || work.ExpectedRevision < 0 || work.DeliveryGeneration < 0 || result.FinishedAt.IsZero() || !knownExecutionSubmitClass(result.Class) {
		return fmt.Errorf("invalid firmware submit result")
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("begin firmware result: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var attemptID, updateID string
	var epoch, revision int64
	err = tx.QueryRow(ctx, `SELECT a.id::text,a.target_update_id::text,a.captured_command_epoch,tu.revision
FROM due_work d JOIN submission_admissions s ON s.target_update_id=d.subject_id
JOIN firmware_update_attempts a ON a.id=s.active_attempt_id JOIN target_updates tu ON tu.id=d.subject_id
WHERE d.id=$1::uuid AND d.command_id=$2::uuid AND d.delivery_generation=$3 AND d.delivery_state='CLAIMED'
AND d.lease_token=$4::uuid AND d.expected_revision=$5 AND d.expected_command_epoch IS NOT DISTINCT FROM $6
AND d.work_type='SUBMIT' FOR UPDATE OF d,s,a,tu`, work.DueWorkID, work.CommandID, work.DeliveryGeneration, work.LeaseToken, work.ExpectedRevision, work.ExpectedCommandEpoch).Scan(&attemptID, &updateID, &epoch, &revision)
	if err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("%w: claimed submit work no longer matches its fence", ErrLeaseConflict)
		}
		return classifyDueWorkError(err, "lock firmware submit result")
	}
	if revision != work.ExpectedRevision || (work.ExpectedCommandEpoch != nil && epoch != *work.ExpectedCommandEpoch) {
		return fmt.Errorf("%w: stale firmware result", ErrLeaseConflict)
	}
	status, nextState, resume, nextWork := resultTransition(result.Class)
	tag, err := tx.Exec(ctx, `UPDATE firmware_update_attempts SET status=$2,result_class=$3,remote_task_reference=$4,response_fingerprint=$5,transport_finished_at=$6,accepted_at=CASE WHEN $2='ACCEPTED' THEN $6 ELSE NULL END,resolved_at=CASE WHEN $2 IN ('REJECTED','PROVEN_ABSENT') THEN $6 ELSE NULL END WHERE id=$1::uuid AND status='ADMITTED'`, attemptID, status, result.Class, nullableExecutionString(result.RemoteTaskReference), nullableExecutionString(result.ResponseFingerprint), result.FinishedAt.UTC())
	if err != nil {
		return fmt.Errorf("record firmware result: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("%w: firmware attempt is no longer admitted", ErrLeaseConflict)
	}
	tag, err = tx.Exec(ctx, `UPDATE target_updates SET state=$2,resume_step=$3,revision=revision+1,updated_at=$4 WHERE id=$1::uuid AND revision=$5 AND command_epoch=$6`, updateID, nextState, nullableExecutionString(resume), now.UTC(), revision, epoch)
	if err != nil {
		return fmt.Errorf("advance firmware target update: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("%w: target update fence changed", ErrLeaseConflict)
	}
	tag, err = tx.Exec(ctx, `UPDATE due_work SET delivery_state='COMPLETED',lease_token=NULL,lease_owner=NULL,lease_expires_at=NULL,updated_at=$5 WHERE id=$1::uuid AND command_id=$2::uuid AND delivery_generation=$3 AND delivery_state='CLAIMED' AND lease_token=$4::uuid`, work.DueWorkID, work.CommandID, work.DeliveryGeneration, work.LeaseToken, now.UTC())
	if err != nil {
		return classifyDueWorkError(err, "complete submit work")
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("%w: submit work no longer claimed", ErrLeaseConflict)
	}
	if nextWork != "" {
		newRevision := revision + 1
		if _, err = tx.Exec(ctx, `INSERT INTO due_work (id,command_id,subject_type,subject_id,work_type,schema_version,not_before,expected_revision,expected_command_epoch)
VALUES (md5($1)::uuid,md5($2)::uuid,'TARGET_UPDATE',$3::uuid,$4,1,$5,$6,$7)`, "fleetforge.execution.work/v1:"+attemptID+":"+nextWork, "fleetforge.execution.command/v1:"+attemptID+":"+nextWork, updateID, nextWork, now.UTC(), newRevision, epoch); err != nil {
			return fmt.Errorf("schedule firmware follow-up: %w", err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return classifyDueWorkError(err, "commit firmware submit result")
	}
	return nil
}

func knownExecutionSubmitClass(class execution.SubmitClass) bool {
	return class == execution.SubmitAccepted || class == execution.SubmitExplicitRejection || class == execution.SubmitProvenPrewriteFailure || class == execution.SubmitPossiblyAccepted || class == execution.SubmitUnsafeResponse
}

func resultTransition(class execution.SubmitClass) (status, state, resume, work string) {
	switch class {
	case execution.SubmitAccepted:
		return "ACCEPTED", "UPDATE_ACCEPTED", "", "TASK_OBSERVE"
	case execution.SubmitExplicitRejection:
		return "REJECTED", "FAILED_TERMINAL", "", ""
	case execution.SubmitProvenPrewriteFailure:
		return "PROVEN_ABSENT", "RETRY_WAIT", "SUBMIT_ADMISSION", ""
	default:
		// NEEDS_ATTENTION cannot carry a resume_step; reconciliation work owns
		// the next decision and cannot create a new submit without proof.
		return "AMBIGUOUS", "NEEDS_ATTENTION", "", "AMBIGUITY_RECONCILE"
	}
}

func nullableExecutionString(v string) any {
	if v == "" {
		return nil
	}
	return v
}
