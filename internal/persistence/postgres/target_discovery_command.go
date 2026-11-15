package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parth10-05/fleetforge/internal/application/access"
	"github.com/parth10-05/fleetforge/internal/application/control"
)

func (adapter ControlTargetAdapter) DiscoverTarget(ctx context.Context, principal access.Principal, meta control.CommandMeta, targetID string, expectedRevision int64) (control.TargetRegistrationResult, error) {
	if adapter.Pool == nil {
		return control.TargetRegistrationResult{}, fmt.Errorf("target discovery persistence is unavailable")
	}
	claim := IdempotencyClaim{ID: meta.ClaimID, ActorID: principal.Subject, Route: meta.Route, RawKey: meta.IdempotencyKey, RequestBody: meta.CanonicalRequest, CommandID: meta.CommandID, OwnerToken: meta.OwnerToken, TTL: 30 * time.Minute}
	if err := ClaimIdempotencyKey(ctx, adapter.Pool, claim); err != nil {
		if !errors.Is(err, ErrIdempotencyInProgress) {
			return control.TargetRegistrationResult{}, err
		}
		_, reference, found, replayErr := GetCompletedIdempotencyRecord(ctx, adapter.Pool, principal.Subject, meta.Route, meta.IdempotencyKey)
		if replayErr != nil || !found {
			if replayErr != nil {
				return control.TargetRegistrationResult{}, replayErr
			}
			return control.TargetRegistrationResult{}, ErrIdempotencyInProgress
		}
		return resolveTargetDiscoveryReplay(ctx, adapter.Pool, reference)
	}
	reference, err := (ResponseReference{Operation: "target-discover", ResourceID: targetID, Revision: expectedRevision + 1, CommandID: meta.CommandID}).Encode()
	if err != nil {
		return control.TargetRegistrationResult{}, fmt.Errorf("encode target discovery response reference: %w", err)
	}
	facts, err := json.Marshal(map[string]any{"target_id": targetID, "work_id": meta.WorkID})
	if err != nil {
		return control.TargetRegistrationResult{}, fmt.Errorf("encode target discovery facts: %w", err)
	}
	if err := ApplyVersionedIdempotentAggregateMutation(ctx, adapter.Pool, IdempotentAggregateMutation{Completion: IdempotencyCompletion{ActorID: principal.Subject, Route: meta.Route, RawKey: meta.IdempotencyKey, ResponseStatus: 202, ResponseReference: reference}, OwnerToken: meta.OwnerToken, Mutation: AggregateMutation{AggregateType: "TARGET", AggregateID: targetID, ExpectedRevision: expectedRevision, ActorID: principal.Subject, CommandID: &meta.CommandID, CorrelationID: meta.CorrelationID, CausationID: meta.CommandID, Action: "TARGET_DISCOVERY_REQUESTED", ReasonCode: "target.discovery.requested", EventSequence: 0, OutboxEventID: meta.OutboxEventID, OutboxEventType: "target.discovery_requested", OutboxSchemaVersion: 1, OutboxPayload: facts, DecisionInputID: meta.DecisionInputID, DecisionSequence: 0, DecisionLogicalTime: time.Now().UTC(), ReducerVersion: "control-api/v1", DecisionPolicyVersion: "target-discovery/v1", NormalizedFacts: facts, DecisionResult: "DISCOVERY_QUEUED", DueWorkID: meta.WorkID, DueWorkCommandID: meta.CommandID, DueWorkSubjectType: "TARGET", DueWorkSubjectID: targetID, DueWorkWorkType: "DISCOVER", DueWorkSchemaVersion: 1, DueWorkNotBefore: time.Now().UTC(), DueWorkExpRevision: expectedRevision + 1}}); err != nil {
		if errors.Is(err, ErrRevisionConflict) {
			return control.TargetRegistrationResult{}, control.ErrTargetRevisionConflict
		}
		return control.TargetRegistrationResult{}, err
	}
	return resolveTargetDiscoveryReplay(ctx, adapter.Pool, reference)
}

func resolveTargetDiscoveryReplay(ctx context.Context, pool *pgxpool.Pool, responseReference string) (control.TargetRegistrationResult, error) {
	reference, err := DecodeResponseReference(responseReference)
	if err != nil || reference.Operation != "target-discover" {
		return control.TargetRegistrationResult{}, fmt.Errorf("invalid target discovery response reference")
	}
	var acceptedAt time.Time
	if err := pool.QueryRow(ctx, "SELECT created_at FROM audit_events WHERE aggregate_type='TARGET' AND aggregate_id=$1::uuid AND aggregate_revision=$2 AND command_id=$3::uuid", reference.ResourceID, reference.Revision, reference.CommandID).Scan(&acceptedAt); err != nil {
		return control.TargetRegistrationResult{}, fmt.Errorf("read target discovery response: %w", err)
	}
	return control.TargetRegistrationResult{TargetID: reference.ResourceID, CommandID: reference.CommandID, Revision: reference.Revision, AcceptedAt: acceptedAt.UTC()}, nil
}

var _ control.TargetDiscoveryStore = ControlTargetAdapter{}
