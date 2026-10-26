package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parth10-05/fleetforge/internal/application/access"
	"github.com/parth10-05/fleetforge/internal/application/control"
	"github.com/parth10-05/fleetforge/internal/domain/inventory"
)

// ControlTargetAdapter is the PostgreSQL implementation of the application
// target command port. It performs no HTTP, simulator, or storage work.
type ControlTargetAdapter struct{ Pool *pgxpool.Pool }

func (adapter ControlTargetAdapter) RegisterTarget(ctx context.Context, principal access.Principal, meta control.CommandMeta, targetID string, registration inventory.Registration) (control.TargetRegistrationResult, error) {
	if adapter.Pool == nil {
		return control.TargetRegistrationResult{}, fmt.Errorf("target command persistence is unavailable")
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
		return adapter.targetReplay(ctx, reference)
	}
	reference, err := (ResponseReference{Operation: "target-register", ResourceID: targetID, Revision: 0, CommandID: meta.CommandID}).Encode()
	if err != nil {
		return control.TargetRegistrationResult{}, fmt.Errorf("encode target registration response reference: %w", err)
	}
	if _, err := ApplyIdempotentTargetRegistration(ctx, adapter.Pool, IdempotentTargetRegistration{TargetID: targetID, CommandID: meta.CommandID, CorrelationID: meta.CorrelationID, OwnerToken: meta.OwnerToken, OutboxEventID: meta.OutboxEventID, DecisionInputID: meta.DecisionInputID, Registration: registration, Completion: IdempotencyCompletion{ActorID: principal.Subject, Route: meta.Route, RawKey: meta.IdempotencyKey, ResponseStatus: 201, ResponseReference: reference}}); err != nil {
		return control.TargetRegistrationResult{}, err
	}
	return adapter.targetReplay(ctx, reference)
}

func (adapter ControlTargetAdapter) targetReplay(ctx context.Context, reference string) (control.TargetRegistrationResult, error) {
	replay, err := ResolveTargetRegistrationReplay(ctx, adapter.Pool, reference)
	if err != nil {
		return control.TargetRegistrationResult{}, err
	}
	return control.TargetRegistrationResult{TargetID: replay.TargetID, CommandID: replay.CommandID, Revision: replay.Revision, AcceptedAt: replay.AcceptedAt}, nil
}

func (adapter ControlTargetAdapter) GetTarget(ctx context.Context, targetID string) (control.TargetRegistrationResult, string, error) {
	if adapter.Pool == nil {
		return control.TargetRegistrationResult{}, "", fmt.Errorf("target projection persistence is unavailable")
	}
	projection, err := GetTargetProjection(ctx, adapter.Pool, targetID)
	if err != nil {
		return control.TargetRegistrationResult{}, "", err
	}
	return control.TargetRegistrationResult{TargetID: projection.ID, Revision: projection.Revision, AcceptedAt: projection.AsOf}, projection.State, nil
}

var _ control.TargetRegistrationStore = ControlTargetAdapter{}
var _ control.TargetProjectionReader = ControlTargetAdapter{}
