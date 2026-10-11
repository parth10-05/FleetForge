package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parth10-05/fleetforge/internal/application/execution"
)

// StartFirmwarePrecheck advances one fenced CANARY_START intent into the
// side-effect-free PRECHECK phase. It records the decision before scheduling
// target I/O, and has no route to SUBMIT or an operation reservation.
func StartFirmwarePrecheck(ctx context.Context, pool *pgxpool.Pool, work execution.WorkFence, now time.Time) error {
	if pool == nil || now.IsZero() || !validExecutionFence(work) {
		return fmt.Errorf("invalid firmware precheck start")
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("begin firmware precheck start: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var updateID, campaignTargetID, campaignID, targetID string
	var revision, epoch int64
	err = tx.QueryRow(ctx, `SELECT tu.id::text,ct.id::text,ct.campaign_id::text,ct.target_id::text,tu.revision,tu.command_epoch
FROM due_work d JOIN target_updates tu ON tu.id=d.subject_id JOIN campaign_targets ct ON ct.id=tu.campaign_target_id JOIN campaigns c ON c.id=ct.campaign_id
WHERE d.id=$1::uuid AND d.command_id=$2::uuid AND d.delivery_generation=$3 AND d.delivery_state='CLAIMED' AND d.lease_token=$4::uuid
AND d.expected_revision=$5 AND d.expected_command_epoch IS NOT DISTINCT FROM $6 AND d.work_type='CANARY_START'
AND tu.state='PENDING' AND c.state IN ('RUNNING_CANARY','RUNNING_WAVES') AND c.command_epoch=tu.command_epoch
FOR UPDATE OF d,tu,ct,c`, work.DueWorkID, work.CommandID, work.DeliveryGeneration, work.LeaseToken, work.ExpectedRevision, work.ExpectedCommandEpoch).Scan(&updateID, &campaignTargetID, &campaignID, &targetID, &revision, &epoch)
	if err == pgx.ErrNoRows {
		return fmt.Errorf("%w: canary start work no longer matches its fence", ErrLeaseConflict)
	}
	if err != nil {
		return classifyDueWorkError(err, "lock firmware precheck start")
	}
	if revision != work.ExpectedRevision || (work.ExpectedCommandEpoch != nil && epoch != *work.ExpectedCommandEpoch) {
		return fmt.Errorf("%w: stale firmware precheck start", ErrLeaseConflict)
	}
	var enrolled string
	if err := tx.QueryRow(ctx, `SELECT campaign_id::text FROM target_enrollments WHERE campaign_target_id=$1::uuid AND target_id=$2::uuid AND campaign_id=$3::uuid AND state='ACTIVE' FOR UPDATE`, campaignTargetID, targetID, campaignID).Scan(&enrolled); err != nil || enrolled != campaignID {
		return fmt.Errorf("%w: active enrollment unavailable for precheck", ErrLeaseConflict)
	}
	tag, err := tx.Exec(ctx, `UPDATE target_updates SET state='PRECHECK',revision=revision+1,updated_at=$3 WHERE id=$1::uuid AND revision=$2 AND command_epoch=$4`, updateID, revision, now.UTC(), epoch)
	if err != nil {
		return classifyDueWorkError(err, "advance target into precheck")
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("%w: target update fence changed", ErrLeaseConflict)
	}
	newRevision := revision + 1
	if _, err = tx.Exec(ctx, `INSERT INTO decision_inputs (id,aggregate_type,aggregate_id,aggregate_revision,decision_sequence,logical_time,reducer_version,policy_version,normalized_facts,result,reason_code)
VALUES (md5($1)::uuid,'TARGET_UPDATE',$2::uuid,$3,0,$4,'target-update/v1','campaign-policy/v1',jsonb_build_object('work_type','CANARY_START','next_work_type','PRECHECK'),'PRECHECK','target.precheck.eligible')`, "fleetforge.precheck.decision/v1:"+work.DueWorkID, updateID, newRevision, now.UTC()); err != nil {
		return classifyDueWorkError(err, "record precheck decision")
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_events (aggregate_type,aggregate_id,aggregate_revision,event_sequence,actor_id,command_id,correlation_id,causation_id,action,reason_code)
VALUES ('TARGET_UPDATE',$1::uuid,$2,0,'system:precheck',$3::uuid,$3::uuid,$3::uuid,'FIRMWARE_PRECHECK_STARTED','target.precheck.eligible')`, updateID, newRevision, work.CommandID); err != nil {
		return classifyDueWorkError(err, "append precheck audit")
	}
	if _, err = tx.Exec(ctx, `INSERT INTO outbox_events (event_id,aggregate_type,aggregate_id,aggregate_revision,event_type,schema_version,payload)
VALUES (md5($1)::uuid,'TARGET_UPDATE',$2::uuid,$3,'target.precheck_started',1,jsonb_build_object('target_update_id',$2,'revision',$3))`, "fleetforge.precheck.outbox/v1:"+work.DueWorkID, updateID, newRevision); err != nil {
		return classifyDueWorkError(err, "insert precheck outbox")
	}
	tag, err = tx.Exec(ctx, `UPDATE due_work SET delivery_state='COMPLETED',lease_token=NULL,lease_owner=NULL,lease_expires_at=NULL,updated_at=$5 WHERE id=$1::uuid AND command_id=$2::uuid AND delivery_generation=$3 AND delivery_state='CLAIMED' AND lease_token=$4::uuid`, work.DueWorkID, work.CommandID, work.DeliveryGeneration, work.LeaseToken, now.UTC())
	if err != nil {
		return classifyDueWorkError(err, "complete canary start work")
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("%w: canary start work no longer claimed", ErrLeaseConflict)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO due_work (id,command_id,subject_type,subject_id,work_type,schema_version,not_before,expected_revision,expected_command_epoch)
VALUES (md5($1)::uuid,md5($2)::uuid,'TARGET_UPDATE',$3::uuid,'PRECHECK',1,$4,$5,$6)`, "fleetforge.precheck.work/v1:"+work.DueWorkID, "fleetforge.precheck.command/v1:"+work.DueWorkID, updateID, now.UTC(), newRevision, epoch); err != nil {
		return classifyDueWorkError(err, "schedule firmware precheck")
	}
	if err = tx.Commit(ctx); err != nil {
		return classifyDueWorkError(err, "commit firmware precheck start")
	}
	return nil
}

func (adapter ExecutionAdapter) StartFirmwarePrecheck(ctx context.Context, work execution.WorkFence) error {
	now := time.Now().UTC()
	if adapter.Now != nil {
		now = adapter.Now().UTC()
	}
	return StartFirmwarePrecheck(ctx, adapter.Pool, work, now)
}

func validExecutionFence(work execution.WorkFence) bool {
	return work.DueWorkID != "" && work.CommandID != "" && work.LeaseToken != "" && work.DeliveryGeneration >= 0 && work.ExpectedRevision >= 0
}
