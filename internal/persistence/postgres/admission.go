// admission.go persists the safety boundary immediately before a future
// firmware submission. It does not call a device, queue, or publisher.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrAdmissionBlocked      = errors.New("submission admission blocked")
	ErrAdmissionDuplicate    = errors.New("submission admission already exists")
	ErrCapacityExhausted     = errors.New("operation capacity exhausted")
	ErrCapacityConfiguration = errors.New("operation capacity configuration conflict")
)

// ScopeCapacity is the immutable policy limit used while admitting one
// operation. Existing durable rows must match this configuration exactly.
type ScopeCapacity struct {
	Level string
	Key   string
	Limit int64
}

// AdmissionRequest contains caller-validated Phase 02 decision data and all
// durable envelopes required to make an admission replayable and auditable.
type AdmissionRequest struct {
	CampaignID, CampaignTargetID, TargetID, ArtifactID, TargetUpdateID string
	ExpectedTargetRevision, CapturedCommandEpoch                       int64
	ExpectedTargetState, NextTargetState                               string
	ResumeStep                                                         *string
	CommandID, Step, AdmissionID, StepAttemptID                        string
	Global, Site, Rack                                                 ScopeCapacity
	ReservationIDs                                                     [3]string // GLOBAL, SITE, RACK
	ActorID, CorrelationID, CausationID, Action, ReasonCode            string
	AuditEventSequence                                                 int64
	OutboxEventID, OutboxEventType                                     string
	OutboxSchemaVersion                                                int
	OutboxPayload                                                      json.RawMessage
	DueWorkID, DueWorkWorkType                                         string
	DueWorkSchemaVersion                                               int
	DueWorkNotBefore                                                   time.Time
	DecisionInputID                                                    string
	DecisionSequence                                                   int64
	DecisionLogicalTime                                                time.Time
	ReducerVersion, DecisionPolicyVersion                              string
	NormalizedFacts                                                    json.RawMessage
	DecisionResult                                                     string
}

// AdmitOperation atomically fences an intended submission, reserves physical
// capacity, and writes all durable evidence. It returns only after commit or
// rollback; no external effect is attempted here.
func AdmitOperation(ctx context.Context, pool *pgxpool.Pool, req AdmissionRequest) error {
	if err := validateAdmissionRequest(req); err != nil {
		return err
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("begin admission transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if err := admitOperationInTx(ctx, tx, req); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		if isSerialization(err) {
			return fmt.Errorf("%w: admission commit", ErrRevisionConflict)
		}
		return fmt.Errorf("commit admission transaction: %w", err)
	}
	return nil
}

func validateAdmissionRequest(r AdmissionRequest) error {
	for _, v := range []string{r.CampaignID, r.CampaignTargetID, r.TargetID, r.ArtifactID, r.TargetUpdateID, r.CommandID, r.Step, r.AdmissionID, r.StepAttemptID, r.ActorID, r.CorrelationID, r.CausationID, r.Action, r.ReasonCode, r.OutboxEventID, r.OutboxEventType, r.DueWorkID, r.DueWorkWorkType, r.DecisionInputID, r.ReducerVersion, r.DecisionPolicyVersion, r.DecisionResult} {
		if v == "" {
			return fmt.Errorf("admission request contains required empty field")
		}
	}
	if r.ExpectedTargetRevision < 0 || r.CapturedCommandEpoch < 0 || r.OutboxSchemaVersion <= 0 || r.DueWorkSchemaVersion <= 0 || r.DecisionLogicalTime.IsZero() || r.DueWorkNotBefore.IsZero() {
		return fmt.Errorf("admission request contains invalid revision, epoch, version, or time")
	}
	if len(r.OutboxPayload) == 0 || len(r.OutboxPayload) > 32768 || len(r.NormalizedFacts) == 0 || len(r.NormalizedFacts) > 32768 || !json.Valid(r.OutboxPayload) || !json.Valid(r.NormalizedFacts) {
		return fmt.Errorf("admission request contains invalid or oversized JSON")
	}
	for _, scope := range []ScopeCapacity{r.Global, r.Site, r.Rack} {
		if scope.Limit <= 0 || scope.Key == "" || (scope.Level != "GLOBAL" && scope.Level != "SITE" && scope.Level != "RACK") || (scope.Level == "GLOBAL" && scope.Key != "GLOBAL") {
			return fmt.Errorf("admission request contains invalid scope capacity")
		}
	}
	for _, id := range r.ReservationIDs {
		if id == "" {
			return fmt.Errorf("admission request contains empty reservation ID")
		}
	}
	return nil
}

func admitOperationInTx(ctx context.Context, tx pgx.Tx, r AdmissionRequest) error {
	var campaignState string
	var epoch int64
	if err := tx.QueryRow(ctx, "SELECT state, command_epoch FROM campaigns WHERE id=$1::uuid FOR UPDATE", r.CampaignID).Scan(&campaignState, &epoch); err != nil {
		if isSerialization(err) {
			return fmt.Errorf("%w: lock campaign for admission", ErrRevisionConflict)
		}
		return fmt.Errorf("lock campaign for admission: %w", err)
	}
	if epoch != r.CapturedCommandEpoch || (campaignState != "RUNNING_CANARY" && campaignState != "RUNNING_WAVES") {
		return fmt.Errorf("%w: campaign is no longer admitting at epoch %d", ErrAdmissionBlocked, r.CapturedCommandEpoch)
	}

	var targetState string
	var targetRevision, targetEpoch int64
	err := tx.QueryRow(ctx, `SELECT tu.state, tu.revision, tu.command_epoch
FROM campaign_targets ct JOIN target_updates tu ON tu.campaign_target_id=ct.id
WHERE ct.id=$1::uuid AND ct.campaign_id=$2::uuid AND ct.target_id=$3::uuid AND ct.artifact_id=$4::uuid AND tu.id=$5::uuid FOR UPDATE`, r.CampaignTargetID, r.CampaignID, r.TargetID, r.ArtifactID, r.TargetUpdateID).Scan(&targetState, &targetRevision, &targetEpoch)
	if err != nil {
		if isSerialization(err) {
			return fmt.Errorf("%w: lock frozen target update for admission", ErrRevisionConflict)
		}
		return fmt.Errorf("lock frozen target update for admission: %w", err)
	}
	if targetState != r.ExpectedTargetState || targetRevision != r.ExpectedTargetRevision || targetEpoch != r.CapturedCommandEpoch {
		return fmt.Errorf("%w: stale target update or epoch", ErrRevisionConflict)
	}

	var enrolledCampaign string
	if err := tx.QueryRow(ctx, "SELECT campaign_id FROM target_enrollments WHERE target_id=$1::uuid AND state='ACTIVE' FOR UPDATE", r.TargetID).Scan(&enrolledCampaign); err != nil {
		return fmt.Errorf("%w: active enrollment unavailable", ErrAdmissionBlocked)
	}
	if enrolledCampaign != r.CampaignID {
		return fmt.Errorf("%w: target belongs to another campaign", ErrAdmissionBlocked)
	}

	for _, scope := range []struct {
		capacity      ScopeCapacity
		reservationID string
	}{{r.Global, r.ReservationIDs[0]}, {r.Site, r.ReservationIDs[1]}, {r.Rack, r.ReservationIDs[2]}} {
		if err := reserveScopeCapacity(ctx, tx, scope.capacity); err != nil {
			return err
		}
		_ = scope.reservationID
	}

	var existing string
	err = tx.QueryRow(ctx, "SELECT id FROM submission_admissions WHERE target_id=$1::uuid AND campaign_id=$2::uuid AND artifact_id=$3::uuid AND step=$4 FOR UPDATE", r.TargetID, r.CampaignID, r.ArtifactID, r.Step).Scan(&existing)
	if err == nil {
		return fmt.Errorf("%w: %s", ErrAdmissionDuplicate, existing)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("lock admission identity: %w", err)
	}
	if _, err = tx.Exec(ctx, "INSERT INTO submission_admissions (id,campaign_id,target_id,artifact_id,target_update_id,command_id,step,captured_command_epoch) VALUES ($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5::uuid,$6::uuid,$7,$8)", r.AdmissionID, r.CampaignID, r.TargetID, r.ArtifactID, r.TargetUpdateID, r.CommandID, r.Step, r.CapturedCommandEpoch); err != nil {
		return classifyAdmissionWrite(err, "insert admission")
	}
	for i, scope := range []ScopeCapacity{r.Global, r.Site, r.Rack} {
		if _, err = tx.Exec(ctx, "INSERT INTO operation_reservations (id,admission_id,target_id,campaign_id,scope_level,scope_key) VALUES ($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5,$6)", r.ReservationIDs[i], r.AdmissionID, r.TargetID, r.CampaignID, scope.Level, scope.Key); err != nil {
			return classifyAdmissionWrite(err, "insert operation reservation")
		}
	}
	if _, err = tx.Exec(ctx, "INSERT INTO step_attempts (id,target_update_id,step,attempt_number,command_id) VALUES ($1::uuid,$2::uuid,$3,1,$4::uuid)", r.StepAttemptID, r.TargetUpdateID, r.Step, r.CommandID); err != nil {
		return classifyAdmissionWrite(err, "insert submission step intent")
	}
	tag, err := tx.Exec(ctx, "UPDATE target_updates SET state=$3, revision=revision+1, resume_step=$4, updated_at=now() WHERE id=$1::uuid AND revision=$2 AND command_epoch=$5", r.TargetUpdateID, r.ExpectedTargetRevision, r.NextTargetState, r.ResumeStep, r.CapturedCommandEpoch)
	if err != nil {
		return classifyAdmissionWrite(err, "advance admitted target update")
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("%w: target update revision or epoch", ErrRevisionConflict)
	}
	newRevision := r.ExpectedTargetRevision + 1
	if _, err = tx.Exec(ctx, "INSERT INTO decision_inputs (id,aggregate_type,aggregate_id,aggregate_revision,decision_sequence,logical_time,reducer_version,policy_version,normalized_facts,result,reason_code) VALUES ($1::uuid,'TARGET_UPDATE',$2::uuid,$3,$4,$5,$6,$7,$8,$9,$10)", r.DecisionInputID, r.TargetUpdateID, newRevision, r.DecisionSequence, r.DecisionLogicalTime, r.ReducerVersion, r.DecisionPolicyVersion, r.NormalizedFacts, r.DecisionResult, r.ReasonCode); err != nil {
		return classifyAdmissionWrite(err, "insert admission decision input")
	}
	if _, err = tx.Exec(ctx, "INSERT INTO audit_events (aggregate_type,aggregate_id,aggregate_revision,event_sequence,actor_id,command_id,correlation_id,causation_id,action,reason_code) VALUES ('TARGET_UPDATE',$1::uuid,$2,$3,$4,$5::uuid,$6::uuid,$7::uuid,$8,$9)", r.TargetUpdateID, newRevision, r.AuditEventSequence, r.ActorID, r.CommandID, r.CorrelationID, r.CausationID, r.Action, r.ReasonCode); err != nil {
		return classifyAdmissionWrite(err, "append admission audit")
	}
	if _, err = tx.Exec(ctx, "INSERT INTO outbox_events (event_id,aggregate_type,aggregate_id,aggregate_revision,event_type,schema_version,payload) VALUES ($1::uuid,'TARGET_UPDATE',$2::uuid,$3,$4,$5,$6)", r.OutboxEventID, r.TargetUpdateID, newRevision, r.OutboxEventType, r.OutboxSchemaVersion, r.OutboxPayload); err != nil {
		return classifyAdmissionWrite(err, "insert admission outbox")
	}
	if _, err = tx.Exec(ctx, "INSERT INTO due_work (id,command_id,subject_type,subject_id,work_type,schema_version,not_before,expected_revision,expected_command_epoch) VALUES ($1::uuid,$2::uuid,'TARGET_UPDATE',$3::uuid,$4,$5,$6,$7,$8)", r.DueWorkID, r.CommandID, r.TargetUpdateID, r.DueWorkWorkType, r.DueWorkSchemaVersion, r.DueWorkNotBefore, newRevision, r.CapturedCommandEpoch); err != nil {
		return classifyAdmissionWrite(err, "insert admission due work")
	}
	return nil
}

func reserveScopeCapacity(ctx context.Context, tx pgx.Tx, s ScopeCapacity) error {
	if _, err := tx.Exec(ctx, "INSERT INTO scope_capacities (scope_level,scope_key,capacity_limit) VALUES ($1,$2,$3) ON CONFLICT (scope_level,scope_key) DO NOTHING", s.Level, s.Key, s.Limit); err != nil {
		return fmt.Errorf("create scope capacity: %w", err)
	}
	var limit, active int64
	if err := tx.QueryRow(ctx, "SELECT capacity_limit,active_reservation_count FROM scope_capacities WHERE scope_level=$1 AND scope_key=$2 FOR UPDATE", s.Level, s.Key).Scan(&limit, &active); err != nil {
		return fmt.Errorf("lock scope capacity: %w", err)
	}
	if limit != s.Limit {
		return fmt.Errorf("%w: %s/%s existing=%d request=%d", ErrCapacityConfiguration, s.Level, s.Key, limit, s.Limit)
	}
	if active >= limit {
		return fmt.Errorf("%w: %s/%s", ErrCapacityExhausted, s.Level, s.Key)
	}
	if _, err := tx.Exec(ctx, "UPDATE scope_capacities SET active_reservation_count=active_reservation_count+1,updated_at=now() WHERE scope_level=$1 AND scope_key=$2 AND active_reservation_count < capacity_limit", s.Level, s.Key); err != nil {
		return fmt.Errorf("increment scope capacity: %w", err)
	}
	return nil
}

func classifyAdmissionWrite(err error, operation string) error {
	if isSerialization(err) {
		return fmt.Errorf("%w: %s", ErrRevisionConflict, operation)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return fmt.Errorf("%w: %s", ErrAdmissionDuplicate, operation)
	}
	return fmt.Errorf("%s: %w", operation, err)
}
