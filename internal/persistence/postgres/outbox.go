package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrOutboxClaimConflict = errors.New("outbox claim conflict")

type ClaimOutboxRequest struct {
	Owner               string
	Now, LeaseExpiresAt time.Time
	Tokens              []string
}
type ClaimedOutboxEvent struct {
	EventID, AggregateType, AggregateID, EventType, ClaimToken string
	AggregateRevision                                          int64
	SchemaVersion                                              int
	Payload                                                    json.RawMessage
	Attempts                                                   int64
}
type OutboxResult struct {
	EventID, ClaimToken string
	Now                 time.Time
	RetryAt             time.Time
	ErrorClass          string
	MaxAttempts         int64
}

// EventPublisher is deliberately narrow. Implementations may perform I/O, but
// PublishClaimedOutbox calls it only after ClaimOutbox has committed.
type EventPublisher interface {
	Publish(context.Context, ClaimedOutboxEvent) error
}

func ClaimOutbox(ctx context.Context, pool *pgxpool.Pool, req ClaimOutboxRequest) ([]ClaimedOutboxEvent, error) {
	if req.Owner == "" || req.Now.IsZero() || !req.LeaseExpiresAt.After(req.Now) || len(req.Tokens) == 0 || len(req.Tokens) > 100 {
		return nil, fmt.Errorf("invalid outbox claim request")
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return nil, fmt.Errorf("begin outbox claim: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	rows, err := tx.Query(ctx, `SELECT event_id,aggregate_type,aggregate_id,aggregate_revision,event_type,schema_version,payload,delivery_attempts FROM outbox_events WHERE delivery_state='PENDING' AND next_attempt_at <= $1 ORDER BY next_attempt_at,created_at,event_id LIMIT $2 FOR UPDATE SKIP LOCKED`, req.Now, len(req.Tokens))
	if err != nil {
		return nil, fmt.Errorf("lock outbox rows: %w", err)
	}
	defer rows.Close()
	items := []ClaimedOutboxEvent{}
	for rows.Next() {
		var item ClaimedOutboxEvent
		if err := rows.Scan(&item.EventID, &item.AggregateType, &item.AggregateID, &item.AggregateRevision, &item.EventType, &item.SchemaVersion, &item.Payload, &item.Attempts); err != nil {
			return nil, fmt.Errorf("scan outbox row: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read outbox rows: %w", err)
	}
	for i := range items {
		items[i].ClaimToken = req.Tokens[i]
		tag, err := tx.Exec(ctx, `UPDATE outbox_events SET delivery_state='CLAIMED',claim_token=$2::uuid,claim_owner=$3,claim_expires_at=$4,delivery_attempts=delivery_attempts+1 WHERE event_id=$1::uuid AND delivery_state='PENDING'`, items[i].EventID, items[i].ClaimToken, req.Owner, req.LeaseExpiresAt)
		if err != nil {
			return nil, classifyOutboxError(err, "claim outbox event")
		}
		if tag.RowsAffected() != 1 {
			return nil, fmt.Errorf("%w: lost outbox row", ErrOutboxClaimConflict)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, classifyOutboxError(err, "commit outbox claim")
	}
	return items, nil
}

func MarkOutboxPublished(ctx context.Context, pool *pgxpool.Pool, result OutboxResult) error {
	if result.EventID == "" || result.ClaimToken == "" || result.Now.IsZero() {
		return fmt.Errorf("invalid outbox published result")
	}
	tag, err := pool.Exec(ctx, `UPDATE outbox_events SET delivery_state='PUBLISHED',published_at=$3,claim_token=NULL,claim_owner=NULL,claim_expires_at=NULL WHERE event_id=$1::uuid AND delivery_state='CLAIMED' AND claim_token=$2::uuid`, result.EventID, result.ClaimToken, result.Now)
	if err != nil {
		return classifyOutboxError(err, "mark outbox published")
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("%w: stale publisher", ErrOutboxClaimConflict)
	}
	return nil
}

func RetryOrQuarantineOutbox(ctx context.Context, pool *pgxpool.Pool, result OutboxResult) error {
	if result.EventID == "" || result.ClaimToken == "" || result.Now.IsZero() || result.ErrorClass == "" || len(result.ErrorClass) > 128 || result.MaxAttempts <= 0 {
		return fmt.Errorf("invalid outbox retry result")
	}
	state := "PENDING"
	next := result.RetryAt
	if result.RetryAt.IsZero() || result.RetryAt.Before(result.Now) {
		return fmt.Errorf("invalid outbox retry time")
	}
	if result.MaxAttempts <= 1 {
		state = "QUARANTINED"
		next = result.Now
	}
	tag, err := pool.Exec(ctx, `UPDATE outbox_events SET delivery_state=$3,next_attempt_at=$4,last_error_class=$5,claim_token=NULL,claim_owner=NULL,claim_expires_at=NULL WHERE event_id=$1::uuid AND delivery_state='CLAIMED' AND claim_token=$2::uuid AND delivery_attempts >= $6`, result.EventID, result.ClaimToken, state, next, result.ErrorClass, result.MaxAttempts)
	if err != nil {
		return classifyOutboxError(err, "retry outbox event")
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("%w: stale publisher or retry budget", ErrOutboxClaimConflict)
	}
	return nil
}

func PublishClaimedOutbox(ctx context.Context, pool *pgxpool.Pool, publisher EventPublisher, claim ClaimOutboxRequest, maxAttempts int64, retryAt func(time.Time) time.Time) error {
	if publisher == nil || retryAt == nil {
		return fmt.Errorf("outbox publisher and retry schedule are required")
	}
	items, err := ClaimOutbox(ctx, pool, claim)
	if err != nil {
		return err
	}
	for _, item := range items {
		result := OutboxResult{EventID: item.EventID, ClaimToken: item.ClaimToken, Now: claim.Now, MaxAttempts: maxAttempts}
		if err := publisher.Publish(ctx, item); err != nil {
			result.ErrorClass = "PUBLISH_FAILED"
			result.RetryAt = retryAt(claim.Now)
			if err := RetryOrQuarantineOutbox(ctx, pool, result); err != nil {
				return err
			}
			continue
		}
		if err := MarkOutboxPublished(ctx, pool, result); err != nil {
			return err
		}
	}
	return nil
}

func classifyOutboxError(err error, operation string) error {
	if isSerialization(err) {
		return fmt.Errorf("%w: %s", ErrOutboxClaimConflict, operation)
	}
	return fmt.Errorf("%s: %w", operation, err)
}
