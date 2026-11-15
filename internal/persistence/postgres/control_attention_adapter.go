package postgres

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parth10-05/fleetforge/internal/application/access"
	"github.com/parth10-05/fleetforge/internal/application/control"
	"github.com/parth10-05/fleetforge/internal/application/execution"
)

// ControlAttentionAdapter applies the existing P05 resolution transition with
// the API idempotency completion in the same serializable transaction.
type ControlAttentionAdapter struct{ Pool *pgxpool.Pool }

func (adapter ControlAttentionAdapter) ResolveAttention(ctx context.Context, principal access.Principal, meta control.CommandMeta, targetUpdateID string, expectedRevision int64, input control.AttentionResolutionCommand) (control.AttentionResolutionResult, error) {
	if adapter.Pool == nil {
		return control.AttentionResolutionResult{}, fmt.Errorf("attention command persistence is unavailable")
	}
	claim := IdempotencyClaim{ID: meta.ClaimID, ActorID: principal.Subject, Route: meta.Route, RawKey: meta.IdempotencyKey, RequestBody: meta.CanonicalRequest, CommandID: meta.CommandID, OwnerToken: meta.OwnerToken, TTL: 30 * time.Minute}
	if err := ClaimIdempotencyKey(ctx, adapter.Pool, claim); err != nil {
		if !errors.Is(err, ErrIdempotencyInProgress) {
			return control.AttentionResolutionResult{}, control.ErrAttentionCommandConflict
		}
		_, reference, found, replayErr := GetCompletedIdempotencyRecord(ctx, adapter.Pool, principal.Subject, meta.Route, meta.IdempotencyKey)
		if replayErr != nil || !found {
			return control.AttentionResolutionResult{}, control.ErrAttentionCommandConflict
		}
		return adapter.attentionReplay(ctx, reference)
	}
	reference, err := (ResponseReference{Operation: "attention-resolution", ResourceID: targetUpdateID, Revision: expectedRevision + 1, CommandID: meta.CommandID}).Encode()
	if err != nil {
		return control.AttentionResolutionResult{}, err
	}
	tx, err := adapter.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return control.AttentionResolutionResult{}, fmt.Errorf("begin idempotent attention resolution: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	keyHash := NormalizeKey(principal.Subject, meta.Route, meta.IdempotencyKey)
	var state, attemptID string
	var epoch int64
	if err := tx.QueryRow(ctx, "SELECT i.state,a.id::text,tu.command_epoch FROM idempotency_records i JOIN target_updates tu ON tu.id=$1::uuid JOIN firmware_update_attempts a ON a.target_update_id=tu.id WHERE i.actor_id=$2 AND i.route=$3 AND i.key_hash=$4 AND i.owner_token=$5::uuid AND tu.revision=$6 AND tu.state='NEEDS_ATTENTION' AND a.status IN ('AMBIGUOUS','NEEDS_ATTENTION') ORDER BY a.started_at DESC LIMIT 1 FOR UPDATE OF i,tu,a", targetUpdateID, principal.Subject, meta.Route, keyHash, meta.OwnerToken, expectedRevision).Scan(&state, &attemptID, &epoch); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return control.AttentionResolutionResult{}, control.ErrAttentionRevisionConflict
		}
		return control.AttentionResolutionResult{}, fmt.Errorf("lock attention command: %w", err)
	}
	if state != string(IdempotencyStateInProgress) {
		return control.AttentionResolutionResult{}, control.ErrAttentionCommandConflict
	}
	now := time.Now().UTC()
	digest := sha256.Sum256(meta.CanonicalRequest)
	command := execution.AdminResolutionCommand{AccessPrincipal: principal, Work: execution.WorkFence{CommandID: meta.CommandID, ExpectedRevision: expectedRevision, ExpectedCommandEpoch: &epoch}, AttemptID: attemptID, Action: input.Action, Reason: input.Reason, ReasonCode: input.ReasonCode, EvidenceFingerprint: fmt.Sprintf("sha256:%x", digest), ReservationDisposition: input.ReservationDisposition, JournalConfirmedAbsent: input.JournalConfirmedAbsent, LateCallerExcluded: input.LateCallerExcluded, LateCallerExclusionMethod: input.LateCallerExclusionMethod, ProofReference: input.Reference, Now: now}
	if err := validReservationDisposition(command); err != nil {
		return control.AttentionResolutionResult{}, control.ErrInvalidAttentionResolution
	}
	if _, err := resolveAttentionTx(ctx, tx, command); err != nil {
		return control.AttentionResolutionResult{}, control.ErrAttentionRevisionConflict
	}
	if err := completeIdempotencyKeyInTx(ctx, tx, IdempotencyCompletion{ActorID: principal.Subject, Route: meta.Route, RawKey: meta.IdempotencyKey, ResponseStatus: 202, ResponseReference: reference}, meta.OwnerToken, keyHash); err != nil {
		return control.AttentionResolutionResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return control.AttentionResolutionResult{}, control.ErrAttentionRevisionConflict
	}
	return adapter.attentionReplay(ctx, reference)
}

func (adapter ControlAttentionAdapter) attentionReplay(ctx context.Context, reference string) (control.AttentionResolutionResult, error) {
	r, err := DecodeResponseReference(reference)
	if err != nil || r.Operation != "attention-resolution" {
		return control.AttentionResolutionResult{}, fmt.Errorf("invalid attention response reference")
	}
	var acceptedAt time.Time
	if err := adapter.Pool.QueryRow(ctx, "SELECT created_at FROM ambiguity_resolutions WHERE command_id=$1::uuid", r.CommandID).Scan(&acceptedAt); err != nil {
		return control.AttentionResolutionResult{}, fmt.Errorf("read attention response: %w", err)
	}
	return control.AttentionResolutionResult{TargetUpdateID: r.ResourceID, CommandID: r.CommandID, Revision: r.Revision, AcceptedAt: acceptedAt.UTC()}, nil
}

var _ control.AttentionResolutionStore = ControlAttentionAdapter{}
