package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parth10-05/fleetforge/internal/domain/inventory"
)

// IdempotentTargetRegistration is the second transaction for an accepted API
// registration claim. Every identifier is allocated before the transaction so
// a retry cannot create an alternate target or durable envelope.
type IdempotentTargetRegistration struct {
	TargetID, CommandID, CorrelationID string
	OwnerToken                         string
	OutboxEventID, DecisionInputID     string
	Registration                       inventory.Registration
	Completion                         IdempotencyCompletion
}

// ApplyIdempotentTargetRegistration writes a newly registered target and its
// audit, decision, outbox, and completion evidence in one transaction.
func ApplyIdempotentTargetRegistration(ctx context.Context, pool *pgxpool.Pool, input IdempotentTargetRegistration) (RegistrationResult, error) {
	if err := validateTargetRegistrationCommand(input); err != nil {
		return RegistrationResult{}, err
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return RegistrationResult{}, fmt.Errorf("begin idempotent target registration: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // intentional best-effort rollback

	keyHash := NormalizeKey(input.Completion.ActorID, input.Completion.Route, input.Completion.RawKey)
	var state string
	if err := tx.QueryRow(ctx, "SELECT state FROM idempotency_records WHERE actor_id=$1 AND route=$2 AND key_hash=$3 AND owner_token=$4::uuid FOR UPDATE", input.Completion.ActorID, input.Completion.Route, keyHash, input.OwnerToken).Scan(&state); err != nil {
		if err == pgx.ErrNoRows {
			return RegistrationResult{}, fmt.Errorf("%w: no matching claim for owner token", ErrIdempotencyConflict)
		}
		return RegistrationResult{}, fmt.Errorf("lock idempotency claim: %w", err)
	}
	if state != string(IdempotencyStateInProgress) {
		return RegistrationResult{}, fmt.Errorf("%w: claim is %s", ErrIdempotencyConflict, state)
	}

	result, err := registerTargetInTx(ctx, tx, input.TargetID, input.Registration)
	if err != nil {
		return RegistrationResult{}, err
	}
	if result.TargetID != input.TargetID {
		return RegistrationResult{}, ErrRegistrationConflict
	}
	if result.Created {
		if err := appendTargetRegistrationEvidence(ctx, tx, input, result.TargetID); err != nil {
			return RegistrationResult{}, err
		}
	}
	if err := completeIdempotencyKeyInTx(ctx, tx, input.Completion, input.OwnerToken, keyHash); err != nil {
		return RegistrationResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		if isSerialization(err) {
			return RegistrationResult{}, fmt.Errorf("%w: serialization during idempotent target registration", ErrRevisionConflict)
		}
		return RegistrationResult{}, fmt.Errorf("commit idempotent target registration: %w", err)
	}
	return result, nil
}

func validateTargetRegistrationCommand(input IdempotentTargetRegistration) error {
	reference, err := DecodeResponseReference(input.Completion.ResponseReference)
	if err != nil || reference.Operation != "target-register" || reference.ResourceID != input.TargetID || reference.CommandID != input.CommandID || reference.Revision != 0 || input.Completion.ResponseStatus != 201 || input.Completion.ActorID == "" || input.Completion.Route == "" || input.Completion.RawKey == "" || input.OwnerToken == "" || input.Registration.ExternalID() == "" {
		return fmt.Errorf("invalid idempotent target registration")
	}
	for _, raw := range []string{input.TargetID, input.CommandID, input.CorrelationID, input.OwnerToken, input.OutboxEventID, input.DecisionInputID} {
		if _, err := uuidParameter(raw); err != nil {
			return fmt.Errorf("invalid idempotent target registration")
		}
	}
	return nil
}

func appendTargetRegistrationEvidence(ctx context.Context, tx pgx.Tx, input IdempotentTargetRegistration, targetID string) error {
	facts, err := json.Marshal(map[string]string{"registration_hash": input.Registration.SemanticHash().String()})
	if err != nil {
		return fmt.Errorf("encode target registration evidence: %w", err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO audit_events (aggregate_type,aggregate_id,aggregate_revision,event_sequence,actor_id,command_id,correlation_id,causation_id,action,reason_code,after_hash) VALUES ('TARGET',$1::uuid,0,0,$2,$3::uuid,$4::uuid,$3::uuid,'TARGET_REGISTERED','target.registration.accepted',$5)", targetID, input.Completion.ActorID, input.CommandID, input.CorrelationID, input.Registration.SemanticHash().String()); err != nil {
		return fmt.Errorf("append target registration audit: %w", err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO decision_inputs (id,aggregate_type,aggregate_id,aggregate_revision,decision_sequence,logical_time,reducer_version,policy_version,normalized_facts,result,reason_code) VALUES ($1::uuid,'TARGET',$2::uuid,0,0,now(),'control-api/v1','target-registration/v1',$3::jsonb,'REGISTERED','target.registration.accepted')", input.DecisionInputID, targetID, string(facts)); err != nil {
		return fmt.Errorf("append target registration decision: %w", err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO outbox_events (event_id,aggregate_type,aggregate_id,aggregate_revision,event_type,schema_version,payload) VALUES ($1::uuid,'TARGET',$2::uuid,0,'target.registered',1,$3::jsonb)", input.OutboxEventID, targetID, string(facts)); err != nil {
		return fmt.Errorf("append target registration outbox: %w", err)
	}
	return nil
}
