package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parth10-05/fleetforge/internal/application/execution"
)

const retryObservationWork = "RETRY_OBSERVATION"

// CommitObservation persists exactly one target observation together with the
// fenced lifecycle move it proves. Repeated observations requeue only their
// own safe read work; they never recreate a SUBMIT command.
func CommitObservation(ctx context.Context, pool *pgxpool.Pool, work execution.WorkFence, decision execution.ObservationDecision, now time.Time) error {
	if pool == nil || now.IsZero() || work.DueWorkID == "" || work.CommandID == "" || work.LeaseToken == "" || work.ExpectedRevision < 0 || work.DeliveryGeneration < 0 || decision.Work != work || !validExecutionObservation(decision.Observation) {
		return fmt.Errorf("invalid execution observation result")
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("begin execution observation: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var attemptID, updateID, campaignTargetID, workType, state string
	var epoch, revision int64
	err = tx.QueryRow(ctx, `SELECT a.id::text,a.target_update_id::text,tu.campaign_target_id::text,d.work_type,tu.state,a.captured_command_epoch,tu.revision
FROM due_work d JOIN submission_admissions s ON s.target_update_id=d.subject_id
JOIN firmware_update_attempts a ON a.id=s.active_attempt_id JOIN target_updates tu ON tu.id=d.subject_id
WHERE d.id=$1::uuid AND d.command_id=$2::uuid AND d.delivery_generation=$3 AND d.delivery_state='CLAIMED'
AND d.lease_token=$4::uuid AND d.expected_revision=$5 AND d.expected_command_epoch IS NOT DISTINCT FROM $6
AND d.work_type IN ('TASK_OBSERVE','REBOOT_OBSERVE','INVENTORY_OBSERVE','HEALTH_OBSERVE') AND a.status='ACCEPTED'
FOR UPDATE OF d,s,a,tu`, work.DueWorkID, work.CommandID, work.DeliveryGeneration, work.LeaseToken, work.ExpectedRevision, work.ExpectedCommandEpoch).Scan(&attemptID, &updateID, &campaignTargetID, &workType, &state, &epoch, &revision)
	if err == pgx.ErrNoRows {
		return fmt.Errorf("%w: claimed observation work no longer matches its fence", ErrLeaseConflict)
	}
	if err != nil {
		return classifyDueWorkError(err, "lock execution observation")
	}
	if revision != work.ExpectedRevision || (work.ExpectedCommandEpoch != nil && epoch != *work.ExpectedCommandEpoch) || decision.Observation.AttemptID != attemptID || decision.Observation.TargetUpdateID != updateID {
		return fmt.Errorf("%w: stale observation result", ErrLeaseConflict)
	}
	if !validObservationRoute(workType, state, decision.NextWorkType) {
		return fmt.Errorf("invalid observation lifecycle route %s/%s/%s", workType, state, decision.NextWorkType)
	}
	_, err = tx.Exec(ctx, `INSERT INTO device_observations (id,target_id,target_update_id,kind,observed_at,received_at,normalized_data,raw_object_reference)
VALUES (md5($1)::uuid,$2::uuid,$3::uuid,$4,$5,$6,$7::jsonb,$8)`, "fleetforge.execution-observation/v1:"+work.DueWorkID+":"+fmt.Sprint(work.DeliveryGeneration), decision.Observation.TargetID, updateID, decision.Observation.Kind, decision.Observation.ObservedAt.UTC(), decision.Observation.ReceivedAt.UTC(), decision.Observation.Data, decision.Observation.ResourcePath)
	if err != nil {
		return fmt.Errorf("persist execution observation: %w", err)
	}
	if workType == "HEALTH_OBSERVE" && decision.NextWorkType == "SUCCEEDED" {
		if _, err = tx.Exec(ctx, `INSERT INTO campaign_health_evidence (id,campaign_target_id,check_name,scheduled_at,observed_at,passed,observation_id)
VALUES (md5($1)::uuid,$2::uuid,'boot',$3,$3,true,md5($4)::uuid) ON CONFLICT (campaign_target_id,check_name,scheduled_at) DO NOTHING`, "fleetforge.execution-health/v1:"+attemptID+":"+fmt.Sprint(work.DeliveryGeneration), campaignTargetID, decision.Observation.ObservedAt.UTC(), "fleetforge.execution-observation/v1:"+work.DueWorkID+":"+fmt.Sprint(work.DeliveryGeneration)); err != nil {
			return fmt.Errorf("persist required boot health evidence: %w", err)
		}
	}
	if decision.NextWorkType == retryObservationWork {
		return requeueObservation(ctx, tx, work, now)
	}
	nextState := observationNextState(workType, decision.NextWorkType)
	tag, err := tx.Exec(ctx, `UPDATE target_updates SET state=$2,resume_step=NULL,revision=revision+1,updated_at=$3 WHERE id=$1::uuid AND revision=$4 AND command_epoch=$5`, updateID, nextState, now.UTC(), revision, epoch)
	if err != nil {
		return fmt.Errorf("advance observation lifecycle: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("%w: target update fence changed", ErrLeaseConflict)
	}
	if decision.NextWorkType == "TERMINAL" {
		if _, err := tx.Exec(ctx, `UPDATE firmware_update_attempts SET status='FAILED_TERMINAL',resolved_at=$2 WHERE id=$1::uuid AND status='ACCEPTED'`, attemptID, now.UTC()); err != nil {
			return fmt.Errorf("mark terminal execution attempt: %w", err)
		}
	}
	tag, err = tx.Exec(ctx, `UPDATE due_work SET delivery_state='COMPLETED',lease_token=NULL,lease_owner=NULL,lease_expires_at=NULL,updated_at=$5 WHERE id=$1::uuid AND command_id=$2::uuid AND delivery_generation=$3 AND delivery_state='CLAIMED' AND lease_token=$4::uuid`, work.DueWorkID, work.CommandID, work.DeliveryGeneration, work.LeaseToken, now.UTC())
	if err != nil {
		return classifyDueWorkError(err, "complete execution observation")
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("%w: observation work no longer claimed", ErrLeaseConflict)
	}
	if decision.NextWorkType == "REBOOT_OBSERVE" || decision.NextWorkType == "INVENTORY_OBSERVE" || decision.NextWorkType == "HEALTH_OBSERVE" {
		notBefore := now.Add(observationDelay(attemptID+":"+decision.NextWorkType, 0))
		if _, err := tx.Exec(ctx, `INSERT INTO due_work (id,command_id,subject_type,subject_id,work_type,schema_version,not_before,expected_revision,expected_command_epoch)
VALUES (md5($1)::uuid,md5($2)::uuid,'TARGET_UPDATE',$3::uuid,$4,1,$5,$6,$7)`, "fleetforge.execution.work/v1:"+attemptID+":"+decision.NextWorkType, "fleetforge.execution.command/v1:"+attemptID+":"+decision.NextWorkType, updateID, decision.NextWorkType, notBefore, revision+1, epoch); err != nil {
			return fmt.Errorf("schedule execution observation: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return classifyDueWorkError(err, "commit execution observation")
	}
	return nil
}

func (adapter ExecutionAdapter) CommitObservation(ctx context.Context, work execution.WorkFence, decision execution.ObservationDecision) error {
	now := time.Now().UTC()
	if adapter.Now != nil {
		now = adapter.Now().UTC()
	}
	return CommitObservation(ctx, adapter.Pool, work, decision, now)
}

func requeueObservation(ctx context.Context, tx pgx.Tx, work execution.WorkFence, now time.Time) error {
	retryAt := now.Add(observationDelay(work.DueWorkID, work.DeliveryGeneration))
	tag, err := tx.Exec(ctx, `UPDATE due_work SET delivery_state='PENDING',not_before=$5,lease_token=NULL,lease_owner=NULL,lease_expires_at=NULL,updated_at=$6 WHERE id=$1::uuid AND command_id=$2::uuid AND delivery_generation=$3 AND delivery_state='CLAIMED' AND lease_token=$4::uuid`, work.DueWorkID, work.CommandID, work.DeliveryGeneration, work.LeaseToken, retryAt, now)
	if err != nil {
		return classifyDueWorkError(err, "requeue execution observation")
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("%w: observation work no longer claimed", ErrLeaseConflict)
	}
	if err := tx.Commit(ctx); err != nil {
		return classifyDueWorkError(err, "commit observation requeue")
	}
	return nil
}

// observationDelay gives each durable observation path a stable delay in the
// five-to-ten-second interval. It spreads target reads without relying on
// process-local randomness; after a worker restart the same fenced work keeps
// the same schedule and cannot become a submission retry.
func observationDelay(identity string, generation int64) time.Duration {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(fmt.Sprintf("%s:%d", identity, generation)))
	return 5*time.Second + time.Duration(hash.Sum32()%5001)*time.Millisecond
}

func validExecutionObservation(observation execution.Observation) bool {
	// Stale observations are durable diagnostic evidence. The application
	// decision layer routes them back to the same safe-read work instead of
	// advancing lifecycle state, so persistence must accept them for that
	// bounded retry path.
	if observation.TargetID == "" || observation.TargetUpdateID == "" || observation.AttemptID == "" || observation.ResourcePath == "" || observation.EvidenceFingerprint == "" || observation.ObservedAt.IsZero() || observation.ReceivedAt.IsZero() || len(observation.Data) == 0 || len(observation.Data) > 32768 {
		return false
	}
	var document map[string]any
	return json.Unmarshal(observation.Data, &document) == nil && document != nil
}

func validObservationRoute(workType, state, next string) bool {
	switch workType {
	case "TASK_OBSERVE":
		return state == "UPDATE_ACCEPTED" && (next == retryObservationWork || next == "REBOOT_OBSERVE" || next == "INVENTORY_OBSERVE" || next == "TERMINAL")
	case "REBOOT_OBSERVE":
		return state == "WAITING_REBOOT" && (next == retryObservationWork || next == "INVENTORY_OBSERVE" || next == "TERMINAL")
	case "INVENTORY_OBSERVE":
		return state == "VERIFYING" && (next == retryObservationWork || next == "HEALTH_OBSERVE" || next == "TERMINAL")
	case "HEALTH_OBSERVE":
		return state == "VERIFYING" && (next == retryObservationWork || next == "SUCCEEDED" || next == "TERMINAL")
	default:
		return false
	}
}

func observationNextState(workType, next string) string {
	if next == "TERMINAL" {
		return "FAILED_TERMINAL"
	}
	if workType == "TASK_OBSERVE" && next != "INVENTORY_OBSERVE" {
		return "WAITING_REBOOT"
	}
	if workType == "REBOOT_OBSERVE" || workType == "INVENTORY_OBSERVE" {
		return "VERIFYING"
	}
	return "SUCCEEDED"
}
