package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parth10-05/fleetforge/internal/application/execution"
)

// CompleteAlreadyCurrent records the target evidence which proves that the
// requested firmware is already active and healthy. It is intentionally a
// separate transaction from submission admission: it consumes the PRECHECK
// lease, creates no attempt or reservation, and cannot schedule SUBMIT work.
func CompleteAlreadyCurrent(ctx context.Context, pool *pgxpool.Pool, command execution.AlreadyCurrentCommand, now time.Time) error {
	if pool == nil || now.IsZero() || !validAlreadyCurrentCommand(command) {
		return fmt.Errorf("invalid already-current completion")
	}
	evidence, err := json.Marshal(map[string]any{
		"current_version":      command.Evidence.CurrentVersion,
		"capable":              command.Evidence.Capable,
		"healthy":              command.Evidence.Healthy,
		"compatible":           command.Evidence.Compatible,
		"evidence_fingerprint": command.Evidence.EvidenceFingerprint,
	})
	if err != nil {
		return fmt.Errorf("encode already-current evidence: %w", err)
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("begin already-current completion: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var campaignID, campaignTargetID, targetID, artifactID, updateID, state string
	var campaignEpoch, updateEpoch, revision int64
	err = tx.QueryRow(ctx, `SELECT ct.campaign_id::text,ct.id::text,ct.target_id::text,ct.artifact_id::text,tu.id::text,tu.state,c.command_epoch,tu.command_epoch,tu.revision
FROM due_work d
JOIN target_updates tu ON tu.id=d.subject_id
JOIN campaign_targets ct ON ct.id=tu.campaign_target_id
JOIN campaigns c ON c.id=ct.campaign_id
WHERE d.id=$1::uuid AND d.command_id=$2::uuid AND d.delivery_generation=$3
  AND d.delivery_state='CLAIMED' AND d.lease_token=$4::uuid AND d.expected_revision=$5
  AND d.expected_command_epoch IS NOT DISTINCT FROM $6 AND d.work_type='PRECHECK'
FOR UPDATE OF d,tu,c,ct`, command.Work.DueWorkID, command.Work.CommandID, command.Work.DeliveryGeneration, command.Work.LeaseToken, command.Work.ExpectedRevision, command.Work.ExpectedCommandEpoch).Scan(
		&campaignID, &campaignTargetID, &targetID, &artifactID, &updateID, &state, &campaignEpoch, &updateEpoch, &revision,
	)
	if err == pgx.ErrNoRows {
		return fmt.Errorf("%w: claimed precheck work no longer matches its fence", ErrLeaseConflict)
	}
	if err != nil {
		return classifyDueWorkError(err, "lock already-current completion")
	}
	if campaignID != command.Snapshot.CampaignID || campaignTargetID != command.Envelope.CampaignTargetID || targetID != command.Snapshot.TargetID || artifactID != command.Snapshot.ArtifactID || updateID != command.Snapshot.TargetUpdateID || state != command.Envelope.ExpectedTargetState || revision != command.Work.ExpectedRevision || updateEpoch != command.Snapshot.CommandEpoch || campaignEpoch != command.Snapshot.CommandEpoch || (command.Work.ExpectedCommandEpoch != nil && updateEpoch != *command.Work.ExpectedCommandEpoch) {
		return fmt.Errorf("%w: stale already-current completion", ErrLeaseConflict)
	}
	var enrolledCampaign string
	if err := tx.QueryRow(ctx, `SELECT campaign_id::text FROM target_enrollments WHERE campaign_target_id=$1::uuid AND target_id=$2::uuid AND campaign_id=$3::uuid AND state='ACTIVE' FOR UPDATE`, campaignTargetID, targetID, campaignID).Scan(&enrolledCampaign); err != nil {
		return fmt.Errorf("%w: active enrollment unavailable for already-current completion", ErrLeaseConflict)
	}
	if enrolledCampaign != campaignID {
		return fmt.Errorf("%w: enrolled campaign changed", ErrLeaseConflict)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO device_observations (id,target_id,target_update_id,kind,observed_at,received_at,normalized_data)
VALUES (md5($1)::uuid,$2::uuid,$3::uuid,'FIRMWARE_PRECHECK',$4,$5,$6::jsonb)`, "fleetforge.firmware-already-current/v1:"+command.Work.DueWorkID+":"+fmt.Sprint(command.Work.DeliveryGeneration), targetID, updateID, command.Evidence.ObservedAt.UTC(), now.UTC(), evidence); err != nil {
		return classifyDueWorkError(err, "persist already-current evidence")
	}
	tag, err := tx.Exec(ctx, `UPDATE target_updates SET state='SUCCEEDED',resume_step=NULL,revision=revision+1,updated_at=$3
WHERE id=$1::uuid AND revision=$2 AND command_epoch=$4`, updateID, revision, now.UTC(), updateEpoch)
	if err != nil {
		return classifyDueWorkError(err, "complete already-current target update")
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("%w: target update fence changed", ErrLeaseConflict)
	}
	newRevision := revision + 1
	if _, err := tx.Exec(ctx, `INSERT INTO decision_inputs (id,aggregate_type,aggregate_id,aggregate_revision,decision_sequence,logical_time,reducer_version,policy_version,normalized_facts,result,reason_code)
VALUES ($1::uuid,'TARGET_UPDATE',$2::uuid,$3,$4,$5,$6,$7,$8::jsonb,$9,$10)`, command.Envelope.DecisionInputID, updateID, newRevision, command.Envelope.DecisionSequence, command.Envelope.DecisionLogicalTime.UTC(), command.Envelope.ReducerVersion, command.Envelope.DecisionPolicyVersion, command.Envelope.NormalizedFacts, command.Envelope.DecisionResult, command.Envelope.ReasonCode); err != nil {
		return classifyDueWorkError(err, "record already-current decision")
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_events (aggregate_type,aggregate_id,aggregate_revision,event_sequence,actor_id,command_id,correlation_id,causation_id,action,reason_code)
VALUES ('TARGET_UPDATE',$1::uuid,$2,$3,$4,$5::uuid,$6::uuid,$7::uuid,$8,$9)`, updateID, newRevision, command.Envelope.AuditEventSequence, command.Envelope.ActorID, command.Work.CommandID, command.Envelope.CorrelationID, command.Envelope.CausationID, command.Envelope.Action, command.Envelope.ReasonCode); err != nil {
		return classifyDueWorkError(err, "append already-current audit")
	}
	if _, err := tx.Exec(ctx, `INSERT INTO outbox_events (event_id,aggregate_type,aggregate_id,aggregate_revision,event_type,schema_version,payload)
VALUES ($1::uuid,'TARGET_UPDATE',$2::uuid,$3,$4,$5,$6::jsonb)`, command.Envelope.OutboxEventID, updateID, newRevision, command.Envelope.OutboxEventType, command.Envelope.OutboxSchemaVersion, command.Envelope.OutboxPayload); err != nil {
		return classifyDueWorkError(err, "insert already-current outbox")
	}
	tag, err = tx.Exec(ctx, `UPDATE due_work SET delivery_state='COMPLETED',lease_token=NULL,lease_owner=NULL,lease_expires_at=NULL,updated_at=$5
WHERE id=$1::uuid AND command_id=$2::uuid AND delivery_generation=$3 AND delivery_state='CLAIMED' AND lease_token=$4::uuid`, command.Work.DueWorkID, command.Work.CommandID, command.Work.DeliveryGeneration, command.Work.LeaseToken, now.UTC())
	if err != nil {
		return classifyDueWorkError(err, "complete already-current precheck work")
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("%w: precheck work no longer claimed", ErrLeaseConflict)
	}
	if err := tx.Commit(ctx); err != nil {
		return classifyDueWorkError(err, "commit already-current completion")
	}
	return nil
}

func (adapter ExecutionAdapter) CompleteAlreadyCurrent(ctx context.Context, command execution.AlreadyCurrentCommand) error {
	now := time.Now().UTC()
	if adapter.Now != nil {
		now = adapter.Now().UTC()
	}
	return CompleteAlreadyCurrent(ctx, adapter.Pool, command, now)
}

func validAlreadyCurrentCommand(command execution.AlreadyCurrentCommand) bool {
	e := command.Envelope
	if command.Work.DueWorkID == "" || command.Work.CommandID == "" || command.Work.LeaseToken == "" || command.Work.ExpectedRevision < 0 || command.Work.DeliveryGeneration < 0 || command.Snapshot.TargetID == "" || command.Snapshot.CampaignID == "" || command.Snapshot.TargetUpdateID == "" || command.Snapshot.ArtifactID == "" || command.Snapshot.ExpectedVersion == "" || command.Snapshot.Revision != command.Work.ExpectedRevision || command.Snapshot.CommandEpoch < 0 || command.Evidence.CurrentVersion != command.Snapshot.ExpectedVersion || !command.Evidence.Healthy || command.Evidence.EvidenceFingerprint == "" || command.Evidence.ObservedAt.IsZero() {
		return false
	}
	if e.CampaignTargetID == "" || e.ExpectedTargetState != "PRECHECK" || e.NextTargetState != "SUCCEEDED" || e.ActorID == "" || e.CorrelationID == "" || e.CausationID == "" || e.Action == "" || e.ReasonCode == "" || e.OutboxEventID == "" || e.OutboxEventType == "" || e.OutboxSchemaVersion <= 0 || len(e.OutboxPayload) == 0 || e.DecisionInputID == "" || e.DecisionLogicalTime.IsZero() || e.ReducerVersion == "" || e.DecisionPolicyVersion == "" || len(e.NormalizedFacts) == 0 || e.DecisionResult != "SUCCEEDED" {
		return false
	}
	return json.Valid(e.OutboxPayload) && json.Valid(e.NormalizedFacts)
}
