package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parth10-05/fleetforge/internal/application/access"
	"github.com/parth10-05/fleetforge/internal/application/control"
	"github.com/parth10-05/fleetforge/internal/domain/campaignplan"
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

var _ control.CampaignCreationStore = ControlCampaignAdapter{}
