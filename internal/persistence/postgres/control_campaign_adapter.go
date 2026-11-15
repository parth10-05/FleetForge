package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parth10-05/fleetforge/internal/application/access"
	"github.com/parth10-05/fleetforge/internal/application/control"
	"github.com/parth10-05/fleetforge/internal/domain/campaign"
	"github.com/parth10-05/fleetforge/internal/domain/campaignplan"
	"github.com/parth10-05/fleetforge/internal/domain/policy"
	"github.com/parth10-05/fleetforge/internal/domain/value"
)

type ControlCampaignAdapter struct{ Pool *pgxpool.Pool }

func (adapter ControlCampaignAdapter) CreateCampaign(ctx context.Context, principal access.Principal, meta control.CommandMeta, command control.CreateCampaignCommand) (control.CampaignCreationResult, error) {
	if adapter.Pool == nil {
		return control.CampaignCreationResult{}, fmt.Errorf("campaign command persistence is unavailable")
	}
	claim := IdempotencyClaim{ID: meta.ClaimID, ActorID: principal.Subject, Route: meta.Route, RawKey: meta.IdempotencyKey, RequestBody: meta.CanonicalRequest, CommandID: meta.CommandID, OwnerToken: meta.OwnerToken, TTL: 30 * time.Minute}
	if err := ClaimIdempotencyKey(ctx, adapter.Pool, claim); err != nil {
		if !errors.Is(err, ErrIdempotencyInProgress) {
			return control.CampaignCreationResult{}, err
		}
		_, reference, found, replayErr := GetCompletedIdempotencyRecord(ctx, adapter.Pool, principal.Subject, meta.Route, meta.IdempotencyKey)
		if replayErr != nil || !found {
			if replayErr != nil {
				return control.CampaignCreationResult{}, replayErr
			}
			return control.CampaignCreationResult{}, ErrIdempotencyInProgress
		}
		return adapter.campaignReplay(ctx, reference)
	}
	reference, err := (ResponseReference{Operation: "campaign-create", ResourceID: command.CampaignID, Revision: 0, CommandID: meta.CommandID}).Encode()
	if err != nil {
		return control.CampaignCreationResult{}, fmt.Errorf("encode campaign creation response reference: %w", err)
	}
	selector, policy, err := campaignDraftJSON(command)
	if err != nil {
		return control.CampaignCreationResult{}, err
	}
	tx, err := adapter.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return control.CampaignCreationResult{}, fmt.Errorf("begin campaign creation: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	keyHash := NormalizeKey(principal.Subject, meta.Route, meta.IdempotencyKey)
	var state string
	if err := tx.QueryRow(ctx, "SELECT state FROM idempotency_records WHERE actor_id=$1 AND route=$2 AND key_hash=$3 AND owner_token=$4::uuid FOR UPDATE", principal.Subject, meta.Route, keyHash, meta.OwnerToken).Scan(&state); err != nil || state != string(IdempotencyStateInProgress) {
		return control.CampaignCreationResult{}, ErrIdempotencyConflict
	}
	if _, err := tx.Exec(ctx, "INSERT INTO campaigns (id,artifact_id,state,policy_version_id) VALUES ($1::uuid,$2::uuid,'DRAFT',$3::uuid)", command.CampaignID, command.ArtifactID, command.PolicyVersionID); err != nil {
		return control.CampaignCreationResult{}, fmt.Errorf("create campaign: %w", err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO campaign_drafts (campaign_id,artifact_id,canonical_selector,policy_json,selection_seed,selection_algorithm,created_by) VALUES ($1::uuid,$2::uuid,$3::jsonb,$4::jsonb,$5,$6,$7)", command.CampaignID, command.ArtifactID, string(selector), string(policy), command.SelectionSeed, campaignplan.SelectionVersionV1, principal.Subject); err != nil {
		return control.CampaignCreationResult{}, fmt.Errorf("create campaign draft: %w", err)
	}
	facts, _ := json.Marshal(map[string]any{"artifact_id": command.ArtifactID, "selection_seed": command.SelectionSeed})
	if _, err := tx.Exec(ctx, "INSERT INTO audit_events (aggregate_type,aggregate_id,aggregate_revision,event_sequence,actor_id,command_id,correlation_id,causation_id,action,reason_code) VALUES ('CAMPAIGN',$1::uuid,0,0,$2,$3::uuid,$4::uuid,$3::uuid,'CAMPAIGN_DRAFT_CREATED','campaign.draft.created')", command.CampaignID, principal.Subject, meta.CommandID, meta.CorrelationID); err != nil {
		return control.CampaignCreationResult{}, fmt.Errorf("append campaign creation audit: %w", err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO decision_inputs (id,aggregate_type,aggregate_id,aggregate_revision,decision_sequence,logical_time,reducer_version,policy_version,normalized_facts,result,reason_code) VALUES ($1::uuid,'CAMPAIGN',$2::uuid,0,0,now(),'control-api/v1',$3,$4::jsonb,'DRAFT','campaign.draft.created')", meta.DecisionInputID, command.CampaignID, campaignplan.PolicyVersionV1, string(facts)); err != nil {
		return control.CampaignCreationResult{}, fmt.Errorf("append campaign creation decision: %w", err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO outbox_events (event_id,aggregate_type,aggregate_id,aggregate_revision,event_type,schema_version,payload) VALUES ($1::uuid,'CAMPAIGN',$2::uuid,0,'campaign.draft_created',1,$3::jsonb)", meta.OutboxEventID, command.CampaignID, string(facts)); err != nil {
		return control.CampaignCreationResult{}, fmt.Errorf("append campaign creation outbox: %w", err)
	}
	if err := completeIdempotencyKeyInTx(ctx, tx, IdempotencyCompletion{ActorID: principal.Subject, Route: meta.Route, RawKey: meta.IdempotencyKey, ResponseStatus: 201, ResponseReference: reference}, meta.OwnerToken, keyHash); err != nil {
		return control.CampaignCreationResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return control.CampaignCreationResult{}, fmt.Errorf("commit campaign creation: %w", err)
	}
	return adapter.campaignReplay(ctx, reference)
}

func campaignDraftJSON(command control.CreateCampaignCommand) ([]byte, []byte, error) {
	selector := command.Selector.CanonicalJSON()
	policy, err := command.Policy.CanonicalJSON()
	if err != nil {
		return nil, nil, fmt.Errorf("canonicalize campaign policy: %w", err)
	}
	return selector, policy, nil
}
func (adapter ControlCampaignAdapter) campaignReplay(ctx context.Context, reference string) (control.CampaignCreationResult, error) {
	r, err := DecodeResponseReference(reference)
	if err != nil || r.Operation != "campaign-create" {
		return control.CampaignCreationResult{}, fmt.Errorf("invalid campaign creation response reference")
	}
	var acceptedAt time.Time
	if err := adapter.Pool.QueryRow(ctx, "SELECT created_at FROM campaigns WHERE id=$1::uuid", r.ResourceID).Scan(&acceptedAt); err != nil {
		return control.CampaignCreationResult{}, fmt.Errorf("read campaign creation response: %w", err)
	}
	return control.CampaignCreationResult{CampaignID: r.ResourceID, CommandID: r.CommandID, Revision: r.Revision, AcceptedAt: acceptedAt.UTC()}, nil
}

func (adapter ControlCampaignAdapter) GetCampaign(ctx context.Context, campaignID string) (control.CampaignProjection, error) {
	projection, err := GetCampaignProjection(ctx, adapter.Pool, campaignID)
	if errors.Is(err, ErrCampaignProjectionNotFound) {
		return control.CampaignProjection{}, control.ErrCampaignNotFound
	}
	if err != nil {
		return control.CampaignProjection{}, err
	}
	return control.CampaignProjection{CampaignID: projection.ID, State: projection.State, Revision: projection.Revision, AsOf: projection.AsOf}, nil
}

func (adapter ControlCampaignAdapter) StartCampaign(ctx context.Context, principal access.Principal, meta control.CommandMeta, campaignID string, expectedRevision int64) (control.CampaignCreationResult, error) {
	if adapter.Pool == nil {
		return control.CampaignCreationResult{}, fmt.Errorf("campaign start persistence is unavailable")
	}
	claim := IdempotencyClaim{ID: meta.ClaimID, ActorID: principal.Subject, Route: meta.Route, RawKey: meta.IdempotencyKey, RequestBody: meta.CanonicalRequest, CommandID: meta.CommandID, OwnerToken: meta.OwnerToken, TTL: 30 * time.Minute}
	if err := ClaimIdempotencyKey(ctx, adapter.Pool, claim); err != nil {
		if !errors.Is(err, ErrIdempotencyInProgress) {
			if errors.Is(err, ErrIdempotencyConflict) {
				return control.CampaignCreationResult{}, control.ErrCampaignCommandConflict
			}
			return control.CampaignCreationResult{}, err
		}
		_, reference, found, replayErr := GetCompletedIdempotencyRecord(ctx, adapter.Pool, principal.Subject, meta.Route, meta.IdempotencyKey)
		if replayErr != nil || !found {
			return control.CampaignCreationResult{}, control.ErrCampaignCommandConflict
		}
		return adapter.campaignStartReplay(ctx, reference)
	}
	epoch, updates, err := adapter.loadCanaryStartInputs(ctx, campaignID)
	if err != nil {
		return control.CampaignCreationResult{}, err
	}
	commandNamespace, err := uuid.Parse(meta.CommandID)
	if err != nil {
		return control.CampaignCreationResult{}, fmt.Errorf("invalid campaign start command")
	}
	intents := make([]CanaryWorkIntent, 0, len(updates))
	for _, updateID := range updates {
		intents = append(intents, CanaryWorkIntent{TargetUpdateID: updateID, CommandID: uuid.NewSHA1(commandNamespace, []byte("fleetforge.campaign-start/v1:"+updateID+":command")).String(), DueWorkID: uuid.NewSHA1(commandNamespace, []byte("fleetforge.campaign-start/v1:"+updateID+":work")).String()})
	}
	reference, err := (ResponseReference{Operation: "campaign-start", ResourceID: campaignID, Revision: expectedRevision + 1, CommandID: meta.CommandID}).Encode()
	if err != nil {
		return control.CampaignCreationResult{}, err
	}
	tx, err := adapter.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return control.CampaignCreationResult{}, fmt.Errorf("begin idempotent campaign start: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	keyHash := NormalizeKey(principal.Subject, meta.Route, meta.IdempotencyKey)
	var state string
	if err := tx.QueryRow(ctx, "SELECT state FROM idempotency_records WHERE actor_id=$1 AND route=$2 AND key_hash=$3 AND owner_token=$4::uuid FOR UPDATE", principal.Subject, meta.Route, keyHash, meta.OwnerToken).Scan(&state); err != nil || state != string(IdempotencyStateInProgress) {
		return control.CampaignCreationResult{}, control.ErrCampaignCommandConflict
	}
	now := time.Now().UTC()
	request := StartCampaignRequest{CampaignID: campaignID, ExpectedRevision: expectedRevision, ExpectedEpoch: epoch, CommandID: meta.CommandID, ActorID: principal.Subject, CorrelationID: meta.CorrelationID, CausationID: meta.CommandID, Now: now, Intents: intents, AuditAction: "CAMPAIGN_CANARY_STARTED", AuditEventSequence: 0, OutboxEventID: meta.OutboxEventID, OutboxSchemaVersion: 1, DecisionInputID: meta.DecisionInputID, DecisionSequence: 0, ReducerVersion: "control-api/v1"}
	if err := startCampaignTx(ctx, tx, request); err != nil {
		if errors.Is(err, ErrCampaignStartConflict) {
			return control.CampaignCreationResult{}, control.ErrCampaignRevisionConflict
		}
		return control.CampaignCreationResult{}, err
	}
	if err := completeIdempotencyKeyInTx(ctx, tx, IdempotencyCompletion{ActorID: principal.Subject, Route: meta.Route, RawKey: meta.IdempotencyKey, ResponseStatus: 202, ResponseReference: reference}, meta.OwnerToken, keyHash); err != nil {
		return control.CampaignCreationResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		if isSerialization(err) {
			return control.CampaignCreationResult{}, control.ErrCampaignRevisionConflict
		}
		return control.CampaignCreationResult{}, fmt.Errorf("commit idempotent campaign start: %w", err)
	}
	return adapter.campaignStartReplay(ctx, reference)
}

func (adapter ControlCampaignAdapter) PauseCampaign(ctx context.Context, principal access.Principal, meta control.CommandMeta, campaignID string, expectedRevision int64) (control.CampaignCreationResult, error) {
	if adapter.Pool == nil {
		return control.CampaignCreationResult{}, fmt.Errorf("campaign pause persistence is unavailable")
	}
	claim := IdempotencyClaim{ID: meta.ClaimID, ActorID: principal.Subject, Route: meta.Route, RawKey: meta.IdempotencyKey, RequestBody: meta.CanonicalRequest, CommandID: meta.CommandID, OwnerToken: meta.OwnerToken, TTL: 30 * time.Minute}
	if err := ClaimIdempotencyKey(ctx, adapter.Pool, claim); err != nil {
		if !errors.Is(err, ErrIdempotencyInProgress) {
			return control.CampaignCreationResult{}, control.ErrCampaignCommandConflict
		}
		_, replay, found, replayErr := GetCompletedIdempotencyRecord(ctx, adapter.Pool, principal.Subject, meta.Route, meta.IdempotencyKey)
		if replayErr != nil || !found {
			return control.CampaignCreationResult{}, control.ErrCampaignCommandConflict
		}
		return adapter.campaignPauseReplay(ctx, replay)
	}
	var state, policyVersion string
	var epoch int64
	var frozenPolicy, frozenTargets *string
	if err := adapter.Pool.QueryRow(ctx, "SELECT state,command_epoch,policy_version_id::text,frozen_policy_hash,frozen_target_snapshot_hash FROM campaigns WHERE id=$1::uuid AND revision=$2", campaignID, expectedRevision).Scan(&state, &epoch, &policyVersion, &frozenPolicy, &frozenTargets); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return control.CampaignCreationResult{}, control.ErrCampaignRevisionConflict
		}
		return control.CampaignCreationResult{}, fmt.Errorf("load campaign pause intent: %w", err)
	}
	commandID, err := value.ParseCommandID(meta.CommandID)
	if err != nil {
		return control.CampaignCreationResult{}, fmt.Errorf("invalid campaign pause command")
	}
	revision, err := value.NewRevision(expectedRevision)
	if err != nil {
		return control.CampaignCreationResult{}, err
	}
	commandEpoch, err := value.NewCommandEpoch(epoch)
	if err != nil {
		return control.CampaignCreationResult{}, err
	}
	decision := campaign.Reduce(campaign.Aggregate{State: campaign.State(state), Revision: revision, CommandEpoch: commandEpoch}, campaign.CommandInput{ID: commandID, Command: campaign.CommandPause, ExpectedRevision: revision, ExpectedEpoch: commandEpoch, Authorization: policy.NewCampaignAuthorization(policy.CampaignGuardPauseAuthorized)})
	if decision.Class != campaign.DecisionAllowed {
		return control.CampaignCreationResult{}, control.ErrCampaignRevisionConflict
	}
	reference, err := (ResponseReference{Operation: "campaign-pause", ResourceID: campaignID, Revision: expectedRevision + 1, CommandID: meta.CommandID}).Encode()
	if err != nil {
		return control.CampaignCreationResult{}, err
	}
	facts, _ := json.Marshal(map[string]string{"campaign_id": campaignID, "state": decision.Aggregate.State.String()})
	mutation := AggregateMutation{AggregateType: "CAMPAIGN", AggregateID: campaignID, ExpectedRevision: expectedRevision, NewState: decision.Aggregate.State.String(), CommandEpoch: int64(decision.Aggregate.CommandEpoch.Uint64()), SuspendedPhase: optionalSuspendedPhase(decision.Aggregate.SuspendedPhase), PolicyVersionID: policyVersion, FrozenPolicyHash: frozenPolicy, FrozenTargetSnapshotHash: frozenTargets, ActorID: principal.Subject, CommandID: &meta.CommandID, CorrelationID: meta.CorrelationID, CausationID: meta.CommandID, Action: "CAMPAIGN_PAUSED", ReasonCode: decision.Reason.String(), EventSequence: 0, OutboxEventID: meta.OutboxEventID, OutboxEventType: "campaign.paused", OutboxSchemaVersion: 1, OutboxPayload: facts, DecisionInputID: meta.DecisionInputID, DecisionSequence: 0, DecisionLogicalTime: time.Now().UTC(), ReducerVersion: "control-api/v1", DecisionPolicyVersion: policyVersion, NormalizedFacts: facts, DecisionResult: decision.Aggregate.State.String()}
	if err := ApplyVersionedIdempotentAggregateMutation(ctx, adapter.Pool, IdempotentAggregateMutation{Completion: IdempotencyCompletion{ActorID: principal.Subject, Route: meta.Route, RawKey: meta.IdempotencyKey, ResponseStatus: 202, ResponseReference: reference}, OwnerToken: meta.OwnerToken, Mutation: mutation}); err != nil {
		if errors.Is(err, ErrRevisionConflict) {
			return control.CampaignCreationResult{}, control.ErrCampaignRevisionConflict
		}
		return control.CampaignCreationResult{}, err
	}
	return adapter.campaignPauseReplay(ctx, reference)
}

func optionalSuspendedPhase(state campaign.State) *string {
	if state == "" {
		return nil
	}
	value := state.String()
	return &value
}

func (adapter ControlCampaignAdapter) campaignPauseReplay(ctx context.Context, reference string) (control.CampaignCreationResult, error) {
	r, err := DecodeResponseReference(reference)
	if err != nil || r.Operation != "campaign-pause" {
		return control.CampaignCreationResult{}, fmt.Errorf("invalid campaign pause response reference")
	}
	var acceptedAt time.Time
	if err := adapter.Pool.QueryRow(ctx, "SELECT created_at FROM audit_events WHERE aggregate_type='CAMPAIGN' AND aggregate_id=$1::uuid AND aggregate_revision=$2 AND command_id=$3::uuid", r.ResourceID, r.Revision, r.CommandID).Scan(&acceptedAt); err != nil {
		return control.CampaignCreationResult{}, fmt.Errorf("read campaign pause response: %w", err)
	}
	return control.CampaignCreationResult{CampaignID: r.ResourceID, CommandID: r.CommandID, Revision: r.Revision, AcceptedAt: acceptedAt.UTC()}, nil
}

func (adapter ControlCampaignAdapter) loadCanaryStartInputs(ctx context.Context, campaignID string) (int64, []string, error) {
	var epoch int64
	if err := adapter.Pool.QueryRow(ctx, "SELECT command_epoch FROM campaigns WHERE id=$1::uuid", campaignID).Scan(&epoch); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil, control.ErrCampaignRevisionConflict
		}
		return 0, nil, fmt.Errorf("load campaign start epoch: %w", err)
	}
	rows, err := adapter.Pool.Query(ctx, "SELECT tu.id::text FROM campaign_targets ct JOIN target_updates tu ON tu.campaign_target_id=ct.id WHERE ct.campaign_id=$1::uuid AND ct.cohort='CANARY' ORDER BY tu.id", campaignID)
	if err != nil {
		return 0, nil, fmt.Errorf("load campaign start targets: %w", err)
	}
	defer rows.Close()
	var updates []string
	for rows.Next() {
		var updateID string
		if err := rows.Scan(&updateID); err != nil {
			return 0, nil, fmt.Errorf("scan campaign start target: %w", err)
		}
		updates = append(updates, updateID)
	}
	if err := rows.Err(); err != nil {
		return 0, nil, fmt.Errorf("read campaign start targets: %w", err)
	}
	return epoch, updates, nil
}

func (adapter ControlCampaignAdapter) campaignStartReplay(ctx context.Context, reference string) (control.CampaignCreationResult, error) {
	r, err := DecodeResponseReference(reference)
	if err != nil || r.Operation != "campaign-start" {
		return control.CampaignCreationResult{}, fmt.Errorf("invalid campaign start response reference")
	}
	var acceptedAt time.Time
	if err := adapter.Pool.QueryRow(ctx, "SELECT created_at FROM audit_events WHERE aggregate_type='CAMPAIGN' AND aggregate_id=$1::uuid AND aggregate_revision=$2 AND command_id=$3::uuid", r.ResourceID, r.Revision, r.CommandID).Scan(&acceptedAt); err != nil {
		return control.CampaignCreationResult{}, fmt.Errorf("read campaign start response: %w", err)
	}
	return control.CampaignCreationResult{CampaignID: r.ResourceID, CommandID: r.CommandID, Revision: r.Revision, AcceptedAt: acceptedAt.UTC()}, nil
}

func (adapter ControlCampaignAdapter) ListCampaignTargets(ctx context.Context, campaignID, afterID string, pageSize int) (control.CampaignTargetPage, error) {
	items, err := ListCampaignTargetProjections(ctx, adapter.Pool, campaignID, afterID, pageSize+1)
	if errors.Is(err, ErrCampaignProjectionNotFound) {
		return control.CampaignTargetPage{}, control.ErrCampaignNotFound
	}
	if err != nil {
		return control.CampaignTargetPage{}, err
	}
	page := control.CampaignTargetPage{Items: make([]control.TargetProjection, 0, min(len(items), pageSize))}
	for _, item := range items[:min(len(items), pageSize)] {
		page.Items = append(page.Items, control.TargetProjection{TargetID: item.ID, State: item.State, Revision: item.Revision, AsOf: item.AsOf})
	}
	if len(items) > pageSize {
		page.NextTargetID = page.Items[len(page.Items)-1].TargetID
	}
	return page, nil
}

func (adapter ControlCampaignAdapter) ListCampaignAudit(ctx context.Context, campaignID string, afterRevision, afterSequence int64, pageSize int) (control.CampaignAuditPage, error) {
	items, err := ListCampaignAuditRecords(ctx, adapter.Pool, campaignID, afterRevision, afterSequence, pageSize+1)
	if errors.Is(err, ErrCampaignProjectionNotFound) {
		return control.CampaignAuditPage{}, control.ErrCampaignNotFound
	}
	if err != nil {
		return control.CampaignAuditPage{}, err
	}
	page := control.CampaignAuditPage{Items: make([]control.CampaignAuditRecord, 0, min(len(items), pageSize))}
	for _, item := range items[:min(len(items), pageSize)] {
		page.Items = append(page.Items, control.CampaignAuditRecord{Action: item.Action, Outcome: item.Outcome, OccurredAt: item.OccurredAt})
	}
	if len(items) > pageSize {
		last := items[pageSize-1]
		page.NextAggregateRevision, page.NextEventSequence, page.HasNext = last.AggregateRevision, last.EventSequence, true
	}
	return page, nil
}

var _ control.CampaignCreationStore = ControlCampaignAdapter{}
var _ control.CampaignProjectionReader = ControlCampaignAdapter{}
var _ control.CampaignTargetProjectionLister = ControlCampaignAdapter{}
var _ control.CampaignAuditLister = ControlCampaignAdapter{}
var _ control.CampaignStartStore = ControlCampaignAdapter{}
var _ control.CampaignPauseStore = ControlCampaignAdapter{}
