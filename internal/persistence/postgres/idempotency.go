// idempotency.go — two-transaction idempotency claim and replay protocol.
//
// The protocol has two separate, short transactions:
//
//  1. ClaimIdempotencyKey: Inserts an IN_PROGRESS record (short claim transaction).
//     Returns ErrIdempotencyConflict if the same key+hash already exists or a
//     different request hash is presented for the same key.
//
//  2. CompleteIdempotencyKey: Updates the record to COMPLETED with a stable
//     response reference (atomic completion transaction).
//
// Hash normalization: the idempotency key is SHA-256 hashed (hex) of the
// normalized "actor:route:rawkey" string; the request hash is SHA-256 of the
// serialized request body. Raw keys are never persisted.
//
// Crash safety: A claim that is never completed leaves the record in
// IN_PROGRESS. Callers that re-present the identical request will find the
// existing IN_PROGRESS record and should poll or return a retry-after response.
// An RECOVERY_REQUIRED state is set by a separate repair path (not Phase 03).
package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrIdempotencyConflict is returned when the same idempotency key exists with
// a different request hash (two different requests sharing a key — rejected) or
// when a duplicate claim arrives and the existing record cannot be safely reused.
var ErrIdempotencyConflict = errors.New("idempotency key conflict")

// ErrIdempotencyInProgress is returned when the same key+hash is already
// claimed and not yet completed. The caller should return a retry-after
// response rather than running the command again.
var ErrIdempotencyInProgress = errors.New("idempotency claim already in progress")

// IdempotencyClaim describes the parameters for a single durable claim.
type IdempotencyClaim struct {
	// ID is a fresh UUID for the new idempotency_records row.
	ID string
	// ActorID identifies the caller system or user.
	ActorID string
	// Route is the canonical API route (must start with '/').
	Route string
	// RawKey is the caller-supplied idempotency key; it is hashed before storage.
	RawKey string
	// RequestBody is the serialized request; its hash is stored for conflict detection.
	RequestBody []byte
	// CommandID is the UUID assigned to the command that will run if this claim wins.
	CommandID string
	// OwnerToken is a fresh UUID token held by the claiming worker.
	OwnerToken string
	// TTL is the duration for which the claim should be held before expiry.
	TTL time.Duration
}

// IdempotencyCompletion describes the result to record when a command finishes.
type IdempotencyCompletion struct {
	ActorID           string
	Route             string
	RawKey            string
	ResponseStatus    int
	ResponseReference string
}

// NormalizeKey returns the hex-encoded SHA-256 hash of "actor:route:rawkey".
// This is the value stored in key_hash; the raw key is never persisted.
func NormalizeKey(actorID, route, rawKey string) string {
	h := sha256.Sum256([]byte(actorID + ":" + route + ":" + rawKey))
	return hex.EncodeToString(h[:])
}

// NormalizeRequestHash returns the hex-encoded SHA-256 hash of the request body.
func NormalizeRequestHash(body []byte) string {
	h := sha256.Sum256(body)
	return hex.EncodeToString(h[:])
}

// ClaimIdempotencyKey executes the first transaction of the two-transaction
// protocol. It inserts an IN_PROGRESS idempotency record. The function returns:
//   - nil — the claim was created; the caller should run the command.
//   - ErrIdempotencyInProgress — the same key+hash already exists and is
//     being processed; caller should return retry-after.
//   - ErrIdempotencyConflict — the same key exists with a different request
//     hash; the operation is rejected.
func ClaimIdempotencyKey(ctx context.Context, pool *pgxpool.Pool, claim IdempotencyClaim) error {
	keyHash := NormalizeKey(claim.ActorID, claim.Route, claim.RawKey)
	requestHash := NormalizeRequestHash(claim.RequestBody)
	expiresAt := time.Now().Add(claim.TTL)

	_, err := pool.Exec(ctx,
		// Q12-related: insert a new idempotency claim.
		"INSERT INTO idempotency_records "+
			"(id, actor_id, route, key_hash, request_hash, command_id, owner_token, claim_expires_at) "+
			"VALUES ($1::uuid, $2, $3, $4, $5, $6::uuid, $7::uuid, $8)",
		claim.ID, claim.ActorID, claim.Route, keyHash, requestHash,
		claim.CommandID, claim.OwnerToken, expiresAt,
	)
	if err == nil {
		return nil
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return fmt.Errorf("insert idempotency claim: %w", err)
	}

	// Unique violation: look up the existing record to distinguish in-progress
	// from same-key-different-hash conflict.
	existing, lookupErr := lookupIdempotencyRecord(ctx, pool, claim.ActorID, claim.Route, keyHash)
	if lookupErr != nil {
		return fmt.Errorf("lookup idempotency record after conflict: %w", lookupErr)
	}

	if existing.requestHash != requestHash {
		return fmt.Errorf("%w: key presented with different request hash", ErrIdempotencyConflict)
	}
	// Same key, same hash — already claimed.
	return ErrIdempotencyInProgress
}

// CompleteIdempotencyKey executes the second transaction: updates the
// IN_PROGRESS record to COMPLETED with a stable response reference.
// The token allows only the original claimant to complete the record.
func CompleteIdempotencyKey(ctx context.Context, pool *pgxpool.Pool, completion IdempotencyCompletion, ownerToken string) error {
	keyHash := NormalizeKey(completion.ActorID, completion.Route, completion.RawKey)

	tag, err := pool.Exec(ctx,
		"UPDATE idempotency_records "+
			"SET state='COMPLETED', completed_at=now(), response_status=$1, response_reference=$2 "+
			"WHERE actor_id=$3 AND route=$4 AND key_hash=$5 AND owner_token=$6::uuid AND state='IN_PROGRESS'",
		completion.ResponseStatus, completion.ResponseReference,
		completion.ActorID, completion.Route, keyHash, ownerToken,
	)
	if err != nil {
		return fmt.Errorf("complete idempotency record: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: no matching IN_PROGRESS record for owner token", ErrIdempotencyConflict)
	}
	return nil
}

// GetCompletedIdempotencyRecord returns the response reference and status for a
// completed idempotency key, enabling stable replay without re-running the command.
func GetCompletedIdempotencyRecord(ctx context.Context, pool *pgxpool.Pool, actorID, route, rawKey string) (status int, responseRef string, found bool, err error) {
	keyHash := NormalizeKey(actorID, route, rawKey)
	var state string
	var respStatus *int
	var respRef *string
	scanErr := pool.QueryRow(ctx,
		"SELECT state, response_status, response_reference FROM idempotency_records "+
			"WHERE actor_id=$1 AND route=$2 AND key_hash=$3",
		actorID, route, keyHash,
	).Scan(&state, &respStatus, &respRef)
	if errors.Is(scanErr, pgx.ErrNoRows) {
		return 0, "", false, nil
	}
	if scanErr != nil {
		return 0, "", false, fmt.Errorf("lookup completed idempotency record: %w", scanErr)
	}
	if state != "COMPLETED" || respStatus == nil || respRef == nil {
		return 0, "", false, nil
	}
	return *respStatus, *respRef, true, nil
}

// idempotencyRecordSnapshot is an internal-only helper used by the conflict path.
type idempotencyRecordSnapshot struct {
	requestHash string
	state       IdempotencyState
}

func lookupIdempotencyRecord(ctx context.Context, pool *pgxpool.Pool, actorID, route, keyHash string) (idempotencyRecordSnapshot, error) {
	var requestHash, state string
	err := pool.QueryRow(ctx,
		"SELECT request_hash, state FROM idempotency_records WHERE actor_id=$1 AND route=$2 AND key_hash=$3",
		actorID, route, keyHash,
	).Scan(&requestHash, &state)
	if err != nil {
		return idempotencyRecordSnapshot{}, err
	}
	parsedState, parseErr := ParseIdempotencyState(state)
	if parseErr != nil {
		return idempotencyRecordSnapshot{}, parseErr
	}
	return idempotencyRecordSnapshot{requestHash: requestHash, state: parsedState}, nil
}
