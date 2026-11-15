// admission.go persists the safety boundary immediately before a future
// firmware submission. It does not call a device, queue, or publisher.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
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

var sha256Pattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// ScopeCapacity is the immutable policy limit used while admitting one
// operation. Existing durable rows must match this configuration exactly.
type ScopeCapacity struct {
	Level string
	Key   string
	Limit int64
}

// AdmissionRequest contains caller-validated decision data and all
// durable envelopes required to make an admission replayable and auditable.
type AdmissionRequest struct {
	CampaignID, CampaignTargetID, TargetID, ArtifactID, TargetUpdateID       string
	ExpectedTargetRevision, CapturedCommandEpoch                             int64
	ExpectedTargetState, NextTargetState                                     string
	ResumeStep                                                               *string
	CommandID, Step, AdmissionID, StepAttemptID                              string
	Global, Site, Rack                                                       ScopeCapacity
	ReservationIDs                                                           [3]string // GLOBAL, SITE, RACK
	ActorID, CorrelationID, CausationID, Action, ReasonCode                  string
	AuditEventSequence                                                       int64
	OutboxEventID, OutboxEventType                                           string
	OutboxSchemaVersion                                                      int
	OutboxPayload                                                            json.RawMessage
	DueWorkID, DueWorkWorkType                                               string
	DueWorkSchemaVersion                                                     int
	DueWorkNotBefore                                                         time.Time
	DecisionInputID                                                          string
	DecisionSequence                                                         int64
	DecisionLogicalTime                                                      time.Time
	ReducerVersion, DecisionPolicyVersion                                    string
	NormalizedFacts                                                          json.RawMessage
	DecisionResult                                                           string
	FirmwareAttempt                                                          *FirmwareAttemptRequest
	PrecheckEvidence                                                         json.RawMessage
	PrecheckObservedAt                                                       time.Time
	RetryParentAttemptID, ClaimedWorkID, ClaimedCommandID, ClaimedLeaseToken string
	ClaimedDeliveryGeneration                                                int64
}

// AdmitRetryOperation is the only persistence path that can create a child
// firmware attempt. It consumes a separately persisted proof authorization;
// it never replays an ambiguous attempt and never acquires a second set of
// physical reservations.
func AdmitRetryOperation(ctx context.Context, pool *pgxpool.Pool, req AdmissionRequest) error {
	if err := validateAdmissionRequest(req); err != nil || req.RetryParentAttemptID == "" {
		return fmt.Errorf("invalid retry admission request")
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("begin retry admission transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	claimedCommandID := req.ClaimedCommandID
	if claimedCommandID == "" {
		claimedCommandID = req.CommandID
	}
	var admissionID, parentID, updateID, targetID string
	var revision, epoch, authorizations, children int64
	err = tx.QueryRow(ctx, `SELECT s.id::text,a.id::text,tu.id::text,s.target_id::text,tu.revision,tu.command_epoch,s.retry_authorizations,
 (SELECT count(*) FROM firmware_update_attempts child WHERE child.submission_admission_id=s.id AND child.parent_attempt_id IS NOT NULL)
FROM due_work d JOIN target_updates tu ON tu.id=d.subject_id JOIN submission_admissions s ON s.target_update_id=tu.id
JOIN firmware_update_attempts a ON a.id=s.active_attempt_id JOIN campaigns c ON c.id=s.campaign_id
WHERE d.id=$1::uuid AND d.command_id=$2::uuid AND d.delivery_generation=$3 AND d.delivery_state='CLAIMED'
AND d.lease_token=$4::uuid AND d.expected_revision=$5 AND d.expected_command_epoch IS NOT DISTINCT FROM $6
AND d.work_type='RETRY_PRECHECK' AND tu.state='RETRY_WAIT' AND a.id=$7::uuid AND a.status='PROVEN_ABSENT'
AND c.state IN ('RUNNING_CANARY','RUNNING_WAVES') AND c.command_epoch=tu.command_epoch
AND (a.result_class='PROVEN_PREWRITE_FAILURE' OR EXISTS (SELECT 1 FROM ambiguity_resolutions r WHERE r.attempt_id=a.id AND r.outcome='PROVEN_ABSENT' AND r.journal_confirmed_absent AND r.late_caller_excluded))
FOR UPDATE OF d,tu,s,a,c`, req.ClaimedWorkID, claimedCommandID, req.ClaimedDeliveryGeneration, req.ClaimedLeaseToken, req.ExpectedTargetRevision, req.CapturedCommandEpoch, req.RetryParentAttemptID).Scan(&admissionID, &parentID, &updateID, &targetID, &revision, &epoch, &authorizations, &children)
	if err != nil {
		return fmt.Errorf("lock retry admission: %w", err)
	}
	if admissionID != req.AdmissionID || updateID != req.TargetUpdateID || targetID != req.TargetID || revision != req.ExpectedTargetRevision || epoch != req.CapturedCommandEpoch || children >= authorizations {
		return fmt.Errorf("%w: retry authorization or fence changed", ErrRevisionConflict)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO device_observations (id,target_id,target_update_id,kind,observed_at,received_at,normalized_data)
VALUES (md5($1)::uuid,$2::uuid,$3::uuid,'FIRMWARE_PRECHECK',$4,$5,$6::jsonb)`, "fleetforge.firmware-retry-precheck/v1:"+req.ClaimedWorkID, req.TargetID, req.TargetUpdateID, req.PrecheckObservedAt.UTC(), req.DecisionLogicalTime.UTC(), req.PrecheckEvidence); err != nil {
		return classifyAdmissionWrite(err, "persist retry precheck evidence")
	}
	if _, err = tx.Exec(ctx, `INSERT INTO step_attempts (id,target_update_id,step,attempt_number,command_id)
VALUES ($1::uuid,$2::uuid,$3::varchar,(SELECT COALESCE(max(attempt_number),0)+1 FROM step_attempts WHERE target_update_id=$2::uuid AND step=$3::varchar),$4::uuid)`, req.StepAttemptID, req.TargetUpdateID, req.Step, req.CommandID); err != nil {
		return classifyAdmissionWrite(err, "insert retry step attempt")
	}
	a := req.FirmwareAttempt
	if _, err = tx.Exec(ctx, `INSERT INTO firmware_update_attempts (id,step_attempt_id,submission_admission_id,parent_attempt_id,target_update_id,campaign_id,target_id,artifact_id,provider,container,object_key,object_version,object_size,sha256_digest,expected_version,update_mechanism,request_fingerprint,captured_command_epoch,admitted_revision)
SELECT $1::uuid,$2::uuid,s.id,$3::uuid,s.target_update_id,s.campaign_id,s.target_id,s.artifact_id,$4,$5,$6,$7,$8,$9,$10,'HTTP_PUSH_V1',$11,$12,$13
FROM submission_admissions s WHERE s.id=$14::uuid`, a.AttemptID, req.StepAttemptID, parentID, a.Provider, a.Container, a.ObjectKey, a.ObjectVersion, a.ObjectSize, a.SHA256Digest, a.ExpectedVersion, a.RequestFingerprint, epoch, revision+1, admissionID); err != nil {
		return classifyAdmissionWrite(err, "insert retry firmware attempt")
	}
	if _, err = tx.Exec(ctx, `UPDATE submission_admissions SET active_attempt_id=$2::uuid WHERE id=$1::uuid AND active_attempt_id=$3::uuid`, admissionID, a.AttemptID, parentID); err != nil {
		return classifyAdmissionWrite(err, "activate retry firmware attempt")
	}
	tag, err := tx.Exec(ctx, `UPDATE target_updates SET state='SUBMITTING',resume_step=NULL,revision=revision+1,updated_at=$3 WHERE id=$1::uuid AND revision=$2 AND command_epoch=$4`, updateID, revision, req.DecisionLogicalTime.UTC(), epoch)
	if err != nil || tag.RowsAffected() != 1 {
		return fmt.Errorf("%w: advance retry admission", ErrRevisionConflict)
	}
	newRevision := revision + 1
	if _, err = tx.Exec(ctx, `INSERT INTO decision_inputs (id,aggregate_type,aggregate_id,aggregate_revision,decision_sequence,logical_time,reducer_version,policy_version,normalized_facts,result,reason_code)
VALUES ($1::uuid,'TARGET_UPDATE',$2::uuid,$3,$4,$5,$6,$7,$8::jsonb,$9,$10)`, req.DecisionInputID, updateID, newRevision, req.DecisionSequence, req.DecisionLogicalTime.UTC(), req.ReducerVersion, req.DecisionPolicyVersion, req.NormalizedFacts, req.DecisionResult, req.ReasonCode); err != nil {
		return classifyAdmissionWrite(err, "record retry admission decision")
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_events (aggregate_type,aggregate_id,aggregate_revision,event_sequence,actor_id,command_id,correlation_id,causation_id,action,reason_code)
VALUES ('TARGET_UPDATE',$1::uuid,$2,$3,$4,$5::uuid,$6::uuid,$7::uuid,$8,$9)`, updateID, newRevision, req.AuditEventSequence, req.ActorID, req.CommandID, req.CorrelationID, req.CausationID, req.Action, req.ReasonCode); err != nil {
		return classifyAdmissionWrite(err, "append retry admission audit")
	}
	if _, err = tx.Exec(ctx, `INSERT INTO outbox_events (event_id,aggregate_type,aggregate_id,aggregate_revision,event_type,schema_version,payload)
VALUES ($1::uuid,'TARGET_UPDATE',$2::uuid,$3,$4,$5,$6::jsonb)`, req.OutboxEventID, updateID, newRevision, req.OutboxEventType, req.OutboxSchemaVersion, req.OutboxPayload); err != nil {
		return classifyAdmissionWrite(err, "insert retry admission outbox")
	}
	if _, err = tx.Exec(ctx, `UPDATE due_work SET delivery_state='COMPLETED',lease_token=NULL,lease_owner=NULL,lease_expires_at=NULL,updated_at=$5 WHERE id=$1::uuid AND command_id=$2::uuid AND delivery_generation=$3 AND delivery_state='CLAIMED' AND lease_token=$4::uuid`, req.ClaimedWorkID, claimedCommandID, req.ClaimedDeliveryGeneration, req.ClaimedLeaseToken, req.DecisionLogicalTime.UTC()); err != nil {
		return classifyAdmissionWrite(err, "complete retry precheck work")
	}
	if _, err = tx.Exec(ctx, `INSERT INTO due_work (id,command_id,subject_type,subject_id,work_type,schema_version,not_before,expected_revision,expected_command_epoch)
VALUES ($1::uuid,md5($2::text || ':retry-submit')::uuid,'TARGET_UPDATE',$3::uuid,'SUBMIT',1,$4,$5,$6)`, req.DueWorkID, req.CommandID, updateID, req.DueWorkNotBefore, newRevision, epoch); err != nil {
		return classifyAdmissionWrite(err, "schedule retry submit work")
	}
	if err = tx.Commit(ctx); err != nil {
		return classifyAdmissionWrite(err, "commit retry admission")
	}
	return nil
}

// FirmwareAttemptRequest is the immutable, target-facing identity captured
// with a P05 admission. It is intentionally separate from generic step
// attempts: one semantic admission may later gain a proof-backed child, but
// an ambiguous attempt can never be silently replayed.
type FirmwareAttemptRequest struct {
	AttemptID                string
	Provider, Container      string
	ObjectKey, ObjectVersion string
	ObjectSize               int64
	SHA256Digest             string
	ExpectedVersion          string
	RequestFingerprint       string
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
	if r.FirmwareAttempt != nil {
		if err := validateFirmwareAttemptRequest(*r.FirmwareAttempt); err != nil {
			return err
		}
		if r.PrecheckObservedAt.IsZero() || len(r.PrecheckEvidence) == 0 || len(r.PrecheckEvidence) > 32768 || !json.Valid(r.PrecheckEvidence) {
			return fmt.Errorf("firmware admission requires valid precheck evidence")
		}
		var evidence map[string]any
		if err := json.Unmarshal(r.PrecheckEvidence, &evidence); err != nil || evidence == nil {
			return fmt.Errorf("firmware admission precheck evidence must be an object")
		}
	}
	return nil
}

func validateFirmwareAttemptRequest(r FirmwareAttemptRequest) error {
	if r.AttemptID == "" || r.Provider == "" || r.Container == "" || r.ObjectKey == "" || r.ObjectVersion == "" || r.ExpectedVersion == "" || r.ObjectSize <= 0 || r.ObjectSize > 16777216 {
		return fmt.Errorf("firmware attempt request contains required empty or invalid field")
	}
	if !sha256Pattern.MatchString(r.SHA256Digest) || !sha256Pattern.MatchString(r.RequestFingerprint) {
		return fmt.Errorf("firmware attempt request contains non-canonical fingerprint")
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
	if r.FirmwareAttempt != nil {
		if _, err := tx.Exec(ctx, `INSERT INTO device_observations (id,target_id,target_update_id,kind,observed_at,received_at,normalized_data)
VALUES (md5($1)::uuid,$2::uuid,$3::uuid,'FIRMWARE_PRECHECK',$4,$5,$6::jsonb)`, "fleetforge.firmware-precheck/v1:"+r.AdmissionID, r.TargetID, r.TargetUpdateID, r.PrecheckObservedAt.UTC(), r.DecisionLogicalTime.UTC(), r.PrecheckEvidence); err != nil {
			return classifyAdmissionWrite(err, "persist firmware precheck evidence")
		}
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
	if r.FirmwareAttempt != nil {
		a := r.FirmwareAttempt
		if _, err = tx.Exec(ctx, `INSERT INTO firmware_update_attempts (id,step_attempt_id,submission_admission_id,target_update_id,campaign_id,target_id,artifact_id,provider,container,object_key,object_version,object_size,sha256_digest,expected_version,update_mechanism,request_fingerprint,captured_command_epoch,admitted_revision)
VALUES ($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5::uuid,$6::uuid,$7::uuid,$8,$9,$10,$11,$12,$13,$14,'HTTP_PUSH_V1',$15,$16,$17)`, a.AttemptID, r.StepAttemptID, r.AdmissionID, r.TargetUpdateID, r.CampaignID, r.TargetID, r.ArtifactID, a.Provider, a.Container, a.ObjectKey, a.ObjectVersion, a.ObjectSize, a.SHA256Digest, a.ExpectedVersion, a.RequestFingerprint, r.CapturedCommandEpoch, r.ExpectedTargetRevision+1); err != nil {
			return classifyAdmissionWrite(err, "insert firmware update attempt")
		}
		if _, err = tx.Exec(ctx, "UPDATE submission_admissions SET initial_attempt_id=$2::uuid,active_attempt_id=$2::uuid WHERE id=$1::uuid AND initial_attempt_id IS NULL AND active_attempt_id IS NULL", r.AdmissionID, a.AttemptID); err != nil {
			return classifyAdmissionWrite(err, "link initial firmware attempt")
		}
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
	if r.ClaimedWorkID != "" || r.ClaimedLeaseToken != "" {
		if r.ClaimedWorkID == "" || r.ClaimedLeaseToken == "" {
			return fmt.Errorf("admission request contains incomplete claimed precheck work")
		}
		claimedCommandID := r.ClaimedCommandID
		if claimedCommandID == "" {
			claimedCommandID = r.CommandID
		}
		tag, err := tx.Exec(ctx, `UPDATE due_work SET delivery_state='COMPLETED',lease_token=NULL,lease_owner=NULL,lease_expires_at=NULL,updated_at=$5 WHERE id=$1::uuid AND command_id=$2::uuid AND delivery_generation=$3 AND delivery_state='CLAIMED' AND lease_token=$4::uuid AND work_type='PRECHECK'`, r.ClaimedWorkID, claimedCommandID, r.ClaimedDeliveryGeneration, r.ClaimedLeaseToken, r.DecisionLogicalTime.UTC())
		if err != nil {
			return classifyAdmissionWrite(err, "complete precheck work")
		}
		if tag.RowsAffected() != 1 {
			return fmt.Errorf("%w: precheck work no longer claimed", ErrRevisionConflict)
		}
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
