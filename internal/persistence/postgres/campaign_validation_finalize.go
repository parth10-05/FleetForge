package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	app "github.com/parth10-05/fleetforge/internal/application/campaignvalidation"
	"github.com/parth10-05/fleetforge/internal/domain/campaign"
	"github.com/parth10-05/fleetforge/internal/domain/campaignplan"
	"github.com/parth10-05/fleetforge/internal/domain/policy"
	"github.com/parth10-05/fleetforge/internal/domain/value"
)

// CampaignValidationFinalizer adapts the application processor to PostgreSQL.
// It never performs evaluation, planning, or external I/O under its transaction.
type CampaignValidationFinalizer struct{ Pool *pgxpool.Pool }

// CampaignValidationInputAdapter reads immutable draft facts outside the
// finalization transaction. The finalizer rechecks every fact before READY.
type CampaignValidationInputAdapter struct{ Pool *pgxpool.Pool }

func (adapter CampaignValidationInputAdapter) LoadCampaignValidationInput(ctx context.Context, work app.Work) (app.ValidationInput, error) {
	if adapter.Pool == nil {
		return app.ValidationInput{}, app.ValidationRetryable{Err: fmt.Errorf("campaign validation persistence is unavailable")}
	}
	var artifactID, policyVersionID, selector, rawPolicy, seed, algorithm, state string
	var revision int64
	err := adapter.Pool.QueryRow(ctx, `SELECT c.artifact_id::text,c.policy_version_id::text,c.state,c.revision,d.canonical_selector::text,d.policy_json::text,d.selection_seed,d.selection_algorithm FROM campaigns c JOIN campaign_drafts d ON d.campaign_id=c.id WHERE c.id=$1::uuid`, work.CampaignID).Scan(&artifactID, &policyVersionID, &state, &revision, &selector, &rawPolicy, &seed, &algorithm)
	if err != nil {
		return app.ValidationInput{}, app.ValidationRetryable{Err: fmt.Errorf("load campaign validation draft: %w", err)}
	}
	if state != "VALIDATING" || revision != work.ExpectedRevision {
		return app.ValidationInput{}, app.ValidationConflict{Err: ErrRevisionConflict}
	}
	var parsed campaignplan.Policy
	if err := json.Unmarshal([]byte(rawPolicy), &parsed); err != nil {
		return app.ValidationInput{}, app.ValidationConflict{Err: fmt.Errorf("decode immutable campaign policy: %w", err)}
	}
	return app.ValidationInput{ArtifactID: artifactID, PolicyVersionID: policyVersionID, Selector: []byte(selector), Policy: parsed, Seed: seed, Algorithm: algorithm, Maximum: 100}, nil
}

func (adapter CampaignValidationFinalizer) FinalizeCampaignValidation(ctx context.Context, request app.FinalizationRequest) error {
	return FinalizeCampaignValidation(ctx, adapter.Pool, request)
}

func (adapter CampaignValidationFinalizer) RejectCampaignValidation(ctx context.Context, request app.RejectionRequest) error {
	return RejectCampaignValidation(ctx, adapter.Pool, request)
}

func FinalizeCampaignValidation(ctx context.Context, pool *pgxpool.Pool, request app.FinalizationRequest) error {
	if pool == nil || request.Now.IsZero() || request.CandidateID == "" || request.DecisionInputID == "" || request.OutboxEventID == "" || request.Snapshot.Hash().IsZero() || len(request.Report) == 0 {
		return fmt.Errorf("invalid campaign validation finalization")
	}
	prepared, err := prepareCampaignPlanPersistence(CampaignPlanPersistenceRequest{CampaignID: request.Work.CampaignID, CandidateID: request.CandidateID, PolicyVersionID: request.PolicyVersionID, ExpectedRevision: request.Work.ExpectedRevision, CanaryWaveID: request.CanaryWaveID, LaterWaveID: request.LaterWaveID, Members: finalizationPlanMembers(request.Members), Plan: request.Plan})
	if err != nil {
		return app.ValidationRejected{Err: err}
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return app.ValidationRetryable{Err: fmt.Errorf("begin campaign validation finalization: %w", err)}
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	actor, correlation, causation, err := validationCommandEvidenceTx(ctx, tx, request.Work)
	if err != nil {
		return classifyValidationFinalizationError(err)
	}
	state, revision, epoch, artifactID, err := lockValidationCampaignTx(ctx, tx, request.Work)
	if err != nil {
		return classifyValidationFinalizationError(err)
	}
	if err := validateValidationPass(state, revision, epoch, request); err != nil {
		return classifyValidationFinalizationError(err)
	}
	if err := validateImmutableDraftTx(ctx, tx, request, artifactID); err != nil {
		return app.ValidationRejected{Err: err}
	}
	if err := persistPreflightCandidateTx(ctx, tx, PreflightCandidateRequest{CandidateID: request.CandidateID, Snapshot: request.Snapshot, Report: request.Report}); err != nil {
		return classifyValidationFinalizationError(err)
	}
	if err := persistCampaignPlanTx(ctx, tx, prepared); err != nil {
		return classifyValidationFinalizationError(err)
	}
	policyHash := sha256.Sum256(request.Plan.PolicyJSON)
	readiness := ReadinessEnrollmentRequest{
		CampaignID: request.Work.CampaignID, CandidateID: request.CandidateID, PolicyVersionID: request.PolicyVersionID, ExpectedRevision: request.Work.ExpectedRevision,
		Targets: finalizationEnrollments(request.Members), FrozenPolicyHash: "sha256:" + hex.EncodeToString(policyHash[:]), FrozenTargetSnapshotHash: request.Snapshot.Hash().String(),
		ActorID: actor, CorrelationID: correlation, CausationID: causation, Action: "CAMPAIGN_VALIDATION_SUCCEEDED", ReasonCode: "campaign.validation.succeeded", EventSequence: 0,
		OutboxEventID: request.OutboxEventID, OutboxEventType: "campaign.validation_succeeded", OutboxSchemaVersion: 1, OutboxPayload: validationFacts(request, "READY"),
		planHash: request.Plan.Hash.String(),
	}
	if err := acquireReadinessEnrollmentTx(ctx, tx, readiness, sortedEnrollmentTargets(readiness.Targets)); err != nil {
		return classifyValidationFinalizationError(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO decision_inputs (id,aggregate_type,aggregate_id,aggregate_revision,decision_sequence,logical_time,reducer_version,policy_version,normalized_facts,result,reason_code) VALUES ($1::uuid,'CAMPAIGN',$2::uuid,$3,0,$4,'campaign-reducer/v1',$5,$6::jsonb,'READY','campaign.validation.succeeded')`, request.DecisionInputID, request.Work.CampaignID, request.Work.ExpectedRevision+1, request.Now.UTC(), request.PolicyVersionID, validationFacts(request, "READY")); err != nil {
		return classifyValidationFinalizationError(fmt.Errorf("append validation decision: %w", err))
	}
	if err := completeDueWorkTx(ctx, tx, DueWorkResult{ID: request.Work.ID, LeaseToken: request.Work.LeaseToken, ExpectedRevision: request.Work.ExpectedRevision, ExpectedCommandEpoch: request.Work.ExpectedCommandEpoch, Now: request.Now.UTC()}); err != nil {
		return classifyValidationFinalizationError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return classifyValidationFinalizationError(err)
	}
	return nil
}

func RejectCampaignValidation(ctx context.Context, pool *pgxpool.Pool, request app.RejectionRequest) error {
	if pool == nil || request.Now.IsZero() || request.ReasonCode == "" || request.DecisionInputID == "" || request.OutboxEventID == "" {
		return fmt.Errorf("invalid campaign validation rejection")
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return app.ValidationRetryable{Err: fmt.Errorf("begin campaign validation rejection: %w", err)}
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	actor, correlation, causation, err := validationCommandEvidenceTx(ctx, tx, request.Work)
	if err != nil {
		return classifyValidationFinalizationError(err)
	}
	state, revision, epoch, _, err := lockValidationCampaignTx(ctx, tx, request.Work)
	if err != nil {
		return classifyValidationFinalizationError(err)
	}
	if err := validateValidationFailure(state, revision, epoch, request); err != nil {
		return classifyValidationFinalizationError(err)
	}
	facts, err := json.Marshal(map[string]string{"reason_code": request.ReasonCode, "state": "DRAFT"})
	if err != nil {
		return fmt.Errorf("encode validation rejection facts: %w", err)
	}
	tag, err := tx.Exec(ctx, `UPDATE campaigns SET state='DRAFT',revision=revision+1,updated_at=$3 WHERE id=$1::uuid AND revision=$2 AND state='VALIDATING'`, request.Work.CampaignID, request.Work.ExpectedRevision, request.Now.UTC())
	if err != nil {
		return classifyValidationFinalizationError(fmt.Errorf("return campaign to draft: %w", err))
	}
	if tag.RowsAffected() != 1 {
		return app.ValidationConflict{Err: ErrRevisionConflict}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO decision_inputs (id,aggregate_type,aggregate_id,aggregate_revision,decision_sequence,logical_time,reducer_version,policy_version,normalized_facts,result,reason_code) VALUES ($1::uuid,'CAMPAIGN',$2::uuid,$3,0,$4,'campaign-reducer/v1',$5,$6::jsonb,'DRAFT','campaign.validation.failed')`, request.DecisionInputID, request.Work.CampaignID, request.Work.ExpectedRevision+1, request.Now.UTC(), "campaign-validation/v1", facts); err != nil {
		return classifyValidationFinalizationError(fmt.Errorf("append validation rejection decision: %w", err))
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_events (aggregate_type,aggregate_id,aggregate_revision,event_sequence,actor_id,command_id,correlation_id,causation_id,action,reason_code) VALUES ('CAMPAIGN',$1::uuid,$2,0,$3,$4::uuid,$5::uuid,$6::uuid,'CAMPAIGN_VALIDATION_FAILED',$7)`, request.Work.CampaignID, request.Work.ExpectedRevision+1, actor, request.DecisionInputID, correlation, causation, request.ReasonCode); err != nil {
		return classifyValidationFinalizationError(fmt.Errorf("append validation rejection audit: %w", err))
	}
	if _, err := tx.Exec(ctx, `INSERT INTO outbox_events (event_id,aggregate_type,aggregate_id,aggregate_revision,event_type,schema_version,payload) VALUES ($1::uuid,'CAMPAIGN',$2::uuid,$3,'campaign.validation_failed',1,$4::jsonb)`, request.OutboxEventID, request.Work.CampaignID, request.Work.ExpectedRevision+1, facts); err != nil {
		return classifyValidationFinalizationError(fmt.Errorf("append validation rejection outbox: %w", err))
	}
	if err := completeDueWorkTx(ctx, tx, DueWorkResult{ID: request.Work.ID, LeaseToken: request.Work.LeaseToken, ExpectedRevision: request.Work.ExpectedRevision, ExpectedCommandEpoch: request.Work.ExpectedCommandEpoch, Now: request.Now.UTC()}); err != nil {
		return classifyValidationFinalizationError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return classifyValidationFinalizationError(err)
	}
	return nil
}

func validationCommandEvidenceTx(ctx context.Context, tx pgx.Tx, work app.Work) (string, string, string, error) {
	var commandID, subjectID, state, leaseToken, actor, correlation, causation string
	var revision int64
	var epoch *int64
	if err := tx.QueryRow(ctx, `SELECT command_id::text,subject_id::text,expected_revision,expected_command_epoch,delivery_state,lease_token::text FROM due_work WHERE id=$1::uuid`, work.ID).Scan(&commandID, &subjectID, &revision, &epoch, &state, &leaseToken); err != nil {
		return "", "", "", fmt.Errorf("read validation work fence: %w", err)
	}
	if commandID != work.CommandID || subjectID != work.CampaignID || revision != work.ExpectedRevision || !sameEpoch(epoch, work.ExpectedCommandEpoch) || state != "CLAIMED" || leaseToken != work.LeaseToken {
		return "", "", "", ErrLeaseConflict
	}
	if err := tx.QueryRow(ctx, `SELECT actor_id,correlation_id::text,causation_id::text FROM audit_events WHERE aggregate_type='CAMPAIGN' AND aggregate_id=$1::uuid AND command_id=$2::uuid AND action='CAMPAIGN_VALIDATION_REQUESTED' ORDER BY id DESC LIMIT 1`, work.CampaignID, work.CommandID).Scan(&actor, &correlation, &causation); err != nil {
		return "", "", "", fmt.Errorf("load validation command evidence: %w", err)
	}
	return actor, correlation, causation, nil
}

func lockValidationCampaignTx(ctx context.Context, tx pgx.Tx, work app.Work) (string, int64, int64, string, error) {
	var state, artifactID string
	var revision, epoch int64
	if err := tx.QueryRow(ctx, `SELECT state,revision,command_epoch,artifact_id::text FROM campaigns WHERE id=$1::uuid FOR UPDATE`, work.CampaignID).Scan(&state, &revision, &epoch, &artifactID); err != nil {
		return "", 0, 0, "", fmt.Errorf("lock validation campaign: %w", err)
	}
	return state, revision, epoch, artifactID, nil
}

func validateImmutableDraftTx(ctx context.Context, tx pgx.Tx, request app.FinalizationRequest, artifactID string) error {
	var draftArtifact string
	if err := tx.QueryRow(ctx, `SELECT d.artifact_id::text FROM campaign_drafts d JOIN campaigns c ON c.id=d.campaign_id WHERE d.campaign_id=$1::uuid AND c.policy_version_id=$2::uuid AND d.canonical_selector=$3::jsonb AND d.policy_json=$4::jsonb AND d.selection_seed=$5 AND d.selection_algorithm=$6 FOR SHARE OF d,c`, request.Work.CampaignID, request.PolicyVersionID, string(request.Snapshot.Selector()), string(request.Plan.PolicyJSON), request.Plan.Seed, request.Plan.Algorithm).Scan(&draftArtifact); err != nil {
		return fmt.Errorf("lock immutable campaign draft: %w", err)
	}
	if draftArtifact != artifactID {
		return ErrCampaignPlanConflict
	}
	return nil
}

func validateValidationPass(state string, revision, epoch int64, request app.FinalizationRequest) error {
	return validationReducerDecision(state, revision, epoch, request.Work.ExpectedCommandEpoch, request.DecisionInputID, campaign.CommandValidationPassed, policy.NewCampaignAuthorization(policy.CampaignGuardPreflightPassed, policy.CampaignGuardSnapshotFrozen))
}

func validateValidationFailure(state string, revision, epoch int64, request app.RejectionRequest) error {
	return validationReducerDecision(state, revision, epoch, request.Work.ExpectedCommandEpoch, request.DecisionInputID, campaign.CommandValidationFailed, policy.NewCampaignAuthorization(policy.CampaignGuardCorrectableFailure))
}

func validationReducerDecision(state string, revision, epoch int64, expectedEpoch *int64, commandID string, command campaign.Command, authorization policy.CampaignAuthorization) error {
	id, err := value.ParseCommandID(commandID)
	if err != nil {
		return err
	}
	rev, err := value.NewRevision(revision)
	if err != nil {
		return err
	}
	commandEpoch, err := value.NewCommandEpoch(epoch)
	if err != nil {
		return err
	}
	expected := commandEpoch
	if expectedEpoch != nil {
		expected, err = value.NewCommandEpoch(*expectedEpoch)
		if err != nil {
			return err
		}
	}
	decision := campaign.Reduce(campaign.Aggregate{State: campaign.State(state), Revision: rev, CommandEpoch: commandEpoch}, campaign.CommandInput{ID: id, Command: command, ExpectedRevision: rev, ExpectedEpoch: expected, Authorization: authorization})
	if decision.Class != campaign.DecisionAllowed {
		return ErrRevisionConflict
	}
	return nil
}

func finalizationPlanMembers(members []app.PlanMember) []CampaignPlanMember {
	result := make([]CampaignPlanMember, 0, len(members))
	for _, member := range members {
		result = append(result, CampaignPlanMember{TargetID: member.TargetID, CampaignTargetID: member.CampaignTargetID, TargetUpdateID: member.TargetUpdateID})
	}
	return result
}

func finalizationEnrollments(members []app.PlanMember) []TargetEnrollmentEntry {
	result := make([]TargetEnrollmentEntry, 0, len(members))
	for _, member := range members {
		result = append(result, TargetEnrollmentEntry{TargetID: member.TargetID, CampaignTargetID: member.CampaignTargetID, EnrollmentID: member.EnrollmentID})
	}
	return result
}

func sortedEnrollmentTargets(entries []TargetEnrollmentEntry) []TargetEnrollmentEntry {
	result := append([]TargetEnrollmentEntry(nil), entries...)
	sort.Slice(result, func(i, j int) bool { return result[i].TargetID < result[j].TargetID })
	return result
}

func validationFacts(request app.FinalizationRequest, state string) json.RawMessage {
	facts, _ := json.Marshal(map[string]string{"candidate_id": request.CandidateID, "plan_hash": request.Plan.Hash.String(), "state": state})
	return facts
}

func sameEpoch(left, right *int64) bool {
	return (left == nil && right == nil) || (left != nil && right != nil && *left == *right)
}

func classifyValidationFinalizationError(err error) error {
	if errors.Is(err, ErrLeaseConflict) || errors.Is(err, ErrRevisionConflict) {
		return app.ValidationConflict{Err: err}
	}
	if errors.Is(err, ErrPreflightInputStale) || errors.Is(err, ErrEnrollmentBlocked) || errors.Is(err, ErrCampaignPlanConflict) {
		return app.ValidationRejected{Err: err}
	}
	if isSerialization(err) {
		return app.ValidationRetryable{Err: err}
	}
	return err
}

var _ app.Finalizer = CampaignValidationFinalizer{}
var _ app.ValidationInputLoader = CampaignValidationInputAdapter{}
