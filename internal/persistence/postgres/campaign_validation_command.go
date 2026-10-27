package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/parth10-05/fleetforge/internal/application/access"
	"github.com/parth10-05/fleetforge/internal/application/control"
)

const campaignValidationWorkType = "CAMPAIGN_VALIDATE"

func (adapter ControlCampaignAdapter) ValidateCampaign(ctx context.Context, principal access.Principal, meta control.CommandMeta, campaignID string, expectedRevision int64) (control.CampaignCreationResult, error) {
	if adapter.Pool == nil {
		return control.CampaignCreationResult{}, fmt.Errorf("campaign validation persistence is unavailable")
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
		return adapter.campaignValidationReplay(ctx, reference)
	}
	var state, policyVersionID string
	var commandEpoch int64
	var frozenPolicy, frozenTargets *string
	if err := adapter.Pool.QueryRow(ctx, "SELECT state,policy_version_id::text,command_epoch,frozen_policy_hash,frozen_target_snapshot_hash FROM campaigns WHERE id=$1::uuid AND revision=$2", campaignID, expectedRevision).Scan(&state, &policyVersionID, &commandEpoch, &frozenPolicy, &frozenTargets); err != nil {
		return control.CampaignCreationResult{}, fmt.Errorf("load campaign validation intent: %w", err)
	}
	if state != "DRAFT" {
		return control.CampaignCreationResult{}, ErrRevisionConflict
	}
	reference, err := (ResponseReference{Operation: "campaign-validate", ResourceID: campaignID, Revision: expectedRevision + 1, CommandID: meta.CommandID}).Encode()
	if err != nil {
		return control.CampaignCreationResult{}, err
	}
	facts, _ := json.Marshal(map[string]any{"campaign_id": campaignID, "work_id": meta.WorkID})
	mutation := AggregateMutation{AggregateType: "CAMPAIGN", AggregateID: campaignID, ExpectedRevision: expectedRevision, NewState: "DRAFT", CommandEpoch: commandEpoch, PolicyVersionID: policyVersionID, FrozenPolicyHash: frozenPolicy, FrozenTargetSnapshotHash: frozenTargets, ActorID: principal.Subject, CommandID: &meta.CommandID, CorrelationID: meta.CorrelationID, CausationID: meta.CommandID, Action: "CAMPAIGN_VALIDATION_REQUESTED", ReasonCode: "campaign.validation.requested", OutboxEventID: meta.OutboxEventID, OutboxEventType: "campaign.validation_requested", OutboxSchemaVersion: 1, OutboxPayload: facts, DecisionInputID: meta.DecisionInputID, DecisionLogicalTime: time.Now().UTC(), ReducerVersion: "control-api/v1", DecisionPolicyVersion: "campaign-validation/v1", NormalizedFacts: facts, DecisionResult: "VALIDATION_QUEUED", DueWorkID: meta.WorkID, DueWorkCommandID: meta.CommandID, DueWorkSubjectType: "CAMPAIGN", DueWorkSubjectID: campaignID, DueWorkWorkType: campaignValidationWorkType, DueWorkSchemaVersion: 1, DueWorkNotBefore: time.Now().UTC(), DueWorkExpRevision: expectedRevision + 1}
	if err := ApplyVersionedIdempotentAggregateMutation(ctx, adapter.Pool, IdempotentAggregateMutation{Completion: IdempotencyCompletion{ActorID: principal.Subject, Route: meta.Route, RawKey: meta.IdempotencyKey, ResponseStatus: 202, ResponseReference: reference}, OwnerToken: meta.OwnerToken, Mutation: mutation}); err != nil {
		return control.CampaignCreationResult{}, err
	}
	return adapter.campaignValidationReplay(ctx, reference)
}

func (adapter ControlCampaignAdapter) campaignValidationReplay(ctx context.Context, reference string) (control.CampaignCreationResult, error) {
	r, err := DecodeResponseReference(reference)
	if err != nil || r.Operation != "campaign-validate" {
		return control.CampaignCreationResult{}, fmt.Errorf("invalid campaign validation response reference")
	}
	var acceptedAt time.Time
	if err := adapter.Pool.QueryRow(ctx, "SELECT created_at FROM audit_events WHERE aggregate_type='CAMPAIGN' AND aggregate_id=$1::uuid AND aggregate_revision=$2 AND command_id=$3::uuid", r.ResourceID, r.Revision, r.CommandID).Scan(&acceptedAt); err != nil {
		return control.CampaignCreationResult{}, fmt.Errorf("read campaign validation response: %w", err)
	}
	return control.CampaignCreationResult{CampaignID: r.ResourceID, CommandID: r.CommandID, Revision: r.Revision, AcceptedAt: acceptedAt.UTC()}, nil
}

var _ control.CampaignValidationStore = ControlCampaignAdapter{}
