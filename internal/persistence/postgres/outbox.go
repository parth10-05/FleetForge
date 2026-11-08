package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parth10-05/fleetforge/internal/application/orchestration"
)

var ErrOutboxClaimConflict = errors.New("outbox claim conflict")

type ClaimOutboxRequest struct {
	Owner               string
	Now, LeaseExpiresAt time.Time
	Tokens              []string
	// WorkTypes limits ClaimWorkWakeups to the work types owned by the
	// consuming binary. An empty slice preserves the generic claim behavior.
	WorkTypes []string
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

// ClaimedWakeup is a typed transport intent reconstructed exclusively from
// authoritative relational columns. Its Envelope is never assembled from an
// untrusted outbox payload.
type ClaimedWakeup struct {
	EventID, ClaimToken string
	Envelope            orchestration.Envelope
	Attempts            int64
}

type RepairWakeupRequest struct {
	Now           time.Time
	DeliveryGrace time.Duration
	Limit         int
	MaxGeneration int64
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
	rows, err := tx.Query(ctx, `SELECT event_id,aggregate_type,aggregate_id,aggregate_revision,event_type,schema_version,payload,delivery_attempts FROM outbox_events WHERE delivery_state='PENDING' AND delivery_kind='DOMAIN_EVENT' AND next_attempt_at <= $1 ORDER BY next_attempt_at,created_at,event_id LIMIT $2 FOR UPDATE SKIP LOCKED`, req.Now, len(req.Tokens))
	if err != nil {
		return nil, classifyOutboxError(err, "lock outbox rows")
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
		return nil, classifyOutboxError(err, "read outbox rows")
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
	next := result.RetryAt
	if result.RetryAt.IsZero() || result.RetryAt.Before(result.Now) {
		return fmt.Errorf("invalid outbox retry time")
	}
	tag, err := pool.Exec(ctx, `UPDATE outbox_events
SET delivery_state=CASE WHEN delivery_attempts >= $6 THEN 'QUARANTINED' ELSE 'PENDING' END,
    next_attempt_at=CASE WHEN delivery_attempts >= $6 THEN $3::timestamptz ELSE $4::timestamptz END,
    last_error_class=$5,claim_token=NULL,claim_owner=NULL,claim_expires_at=NULL
WHERE event_id=$1::uuid AND delivery_state='CLAIMED' AND claim_token=$2::uuid`, result.EventID, result.ClaimToken, result.Now, next, result.ErrorClass, result.MaxAttempts)
	if err != nil {
		return classifyOutboxError(err, "retry outbox event")
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("%w: stale publisher or retry budget", ErrOutboxClaimConflict)
	}
	return nil
}

// ClaimWorkWakeups claims only typed wakeup rows in deterministic order. The
// transaction commits before the caller is permitted to invoke the queue.
func ClaimWorkWakeups(ctx context.Context, pool *pgxpool.Pool, req ClaimOutboxRequest) ([]ClaimedWakeup, error) {
	if req.Owner == "" || req.Now.IsZero() || !req.LeaseExpiresAt.After(req.Now) || len(req.Tokens) == 0 || len(req.Tokens) > 100 {
		return nil, fmt.Errorf("invalid wakeup claim request")
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return nil, fmt.Errorf("begin wakeup claim: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	rows, err := tx.Query(ctx, `SELECT o.event_id,o.command_id,o.delivery_generation,o.queue_class,o.delivery_attempts,
	 d.id,d.work_type,d.subject_type,d.subject_id,d.expected_revision,d.expected_command_epoch,d.not_before
FROM outbox_events o JOIN due_work d ON d.id=o.due_work_id AND d.command_id=o.command_id AND d.delivery_generation=o.delivery_generation
WHERE o.delivery_state='PENDING' AND o.delivery_kind='WORK_WAKEUP' AND o.next_attempt_at <= $1
  AND (cardinality($3::text[]) = 0 OR d.work_type = ANY($3::text[]))
ORDER BY o.next_attempt_at,o.created_at,o.event_id LIMIT $2 FOR UPDATE OF o SKIP LOCKED`, req.Now, len(req.Tokens), req.WorkTypes)
	if err != nil {
		return nil, classifyOutboxError(err, "lock wakeup intents")
	}
	defer rows.Close()
	items := make([]ClaimedWakeup, 0, len(req.Tokens))
	for rows.Next() {
		var item ClaimedWakeup
		var queueClass string
		if err := rows.Scan(&item.EventID, &item.Envelope.CommandID, &item.Envelope.DeliveryGeneration, &queueClass, &item.Attempts,
			&item.Envelope.DueWorkID, &item.Envelope.WorkType, &item.Envelope.SubjectType, &item.Envelope.SubjectID, &item.Envelope.ExpectedRevision, &item.Envelope.ExpectedCommandEpoch, &item.Envelope.NotBefore); err != nil {
			return nil, fmt.Errorf("scan wakeup intent: %w", err)
		}
		item.Envelope.SchemaVersion = orchestration.WakeupSchemaV1
		item.Envelope.QueueClass = orchestration.QueueClass(queueClass)
		// PostgreSQL returns timestamptz values using the connection's location,
		// which can be time.Local even when the value has a UTC offset. Wakeup
		// envelopes require a canonical UTC timestamp at the transport boundary.
		item.Envelope.NotBefore = item.Envelope.NotBefore.UTC()
		if err := item.Envelope.Validate(); err != nil {
			return nil, fmt.Errorf("invalid persisted wakeup %s: %w", item.EventID, err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, classifyOutboxError(err, "read wakeup intents")
	}
	for i := range items {
		items[i].ClaimToken = req.Tokens[i]
		tag, err := tx.Exec(ctx, `UPDATE outbox_events SET delivery_state='CLAIMED',claim_token=$2::uuid,claim_owner=$3,claim_expires_at=$4,delivery_attempts=delivery_attempts+1 WHERE event_id=$1::uuid AND delivery_state='PENDING' AND delivery_kind='WORK_WAKEUP'`, items[i].EventID, items[i].ClaimToken, req.Owner, req.LeaseExpiresAt)
		if err != nil {
			return nil, classifyOutboxError(err, "claim wakeup intent")
		}
		if tag.RowsAffected() != 1 {
			return nil, fmt.Errorf("%w: lost wakeup intent", ErrOutboxClaimConflict)
		}
		items[i].Attempts++
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, classifyOutboxError(err, "commit wakeup claim")
	}
	return items, nil
}

func MarkWorkWakeupPublished(ctx context.Context, pool *pgxpool.Pool, result OutboxResult) error {
	if result.EventID == "" || result.ClaimToken == "" || result.Now.IsZero() {
		return fmt.Errorf("invalid wakeup published result")
	}
	tag, err := pool.Exec(ctx, `UPDATE outbox_events SET delivery_state='PUBLISHED',published_at=$3,claim_token=NULL,claim_owner=NULL,claim_expires_at=NULL WHERE event_id=$1::uuid AND delivery_state='CLAIMED' AND delivery_kind='WORK_WAKEUP' AND claim_token=$2::uuid`, result.EventID, result.ClaimToken, result.Now)
	if err != nil {
		return classifyOutboxError(err, "mark wakeup published")
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("%w: stale wakeup publisher", ErrOutboxClaimConflict)
	}
	return nil
}

// QuarantineWorkWakeup permanently closes a claimed work-wakeup intent and
// atomically records bounded, deduplicated evidence of the publisher rejection.
func QuarantineWorkWakeup(ctx context.Context, pool *pgxpool.Pool, result OutboxResult) error {
	if result.EventID == "" || result.ClaimToken == "" || result.Now.IsZero() {
		return fmt.Errorf("invalid wakeup quarantine result")
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("begin wakeup quarantine: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var dueWorkID, commandID string
	if err := tx.QueryRow(ctx, `UPDATE outbox_events SET delivery_state='QUARANTINED',next_attempt_at=$3,last_error_class='PERMANENT_PUBLISH_REJECTION',claim_token=NULL,claim_owner=NULL,claim_expires_at=NULL WHERE event_id=$1::uuid AND delivery_state='CLAIMED' AND delivery_kind='WORK_WAKEUP' AND claim_token=$2::uuid RETURNING due_work_id::text,command_id::text`, result.EventID, result.ClaimToken, result.Now).Scan(&dueWorkID, &commandID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: stale wakeup publisher", ErrOutboxClaimConflict)
		}
		return classifyOutboxError(err, "quarantine wakeup")
	}
	key := fmt.Sprintf("%x", sha256.Sum256([]byte("PERMANENT_PUBLISH_REJECTION:"+result.EventID)))
	if _, err := tx.Exec(ctx, `INSERT INTO wakeup_quarantine (id,deduplication_key,due_work_id,command_id,reason_code,first_observed_at,last_observed_at) VALUES (md5($1)::uuid,$2,$3::uuid,$4::uuid,'PERMANENT_PUBLISH_REJECTION',$5,$5) ON CONFLICT (deduplication_key) DO UPDATE SET last_observed_at=EXCLUDED.last_observed_at,occurrences=wakeup_quarantine.occurrences+1`, "fleetforge.wakeup-quarantine/v1:"+key, key, dueWorkID, commandID, result.Now); err != nil {
		return fmt.Errorf("record wakeup quarantine: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return classifyOutboxError(err, "commit wakeup quarantine")
	}
	return nil
}

// PublishClaimedWakeups performs transport I/O only after each claim has
// committed. Unknown outcomes retry the same durable row and generation.
func PublishClaimedWakeups(ctx context.Context, pool *pgxpool.Pool, publisher orchestration.WakeupPublisher, claim ClaimOutboxRequest, maxAttempts int64, retryAt func(ClaimedWakeup, time.Time) time.Time) error {
	if publisher == nil || retryAt == nil || maxAttempts <= 0 {
		return fmt.Errorf("wakeup publisher, retry schedule, and attempt budget are required")
	}
	items, err := ClaimWorkWakeups(ctx, pool, claim)
	if err != nil {
		return err
	}
	for _, item := range items {
		result := OutboxResult{EventID: item.EventID, ClaimToken: item.ClaimToken, Now: claim.Now, MaxAttempts: maxAttempts}
		outcome, publishErr := publisher.Publish(ctx, item.Envelope)
		if publishErr != nil || outcome == orchestration.PublishRejectedRetryable || outcome == orchestration.PublishOutcomeUnknown {
			result.ErrorClass, result.RetryAt = "PUBLISH_RETRYABLE", retryAt(item, claim.Now)
			if err := RetryOrQuarantineOutbox(ctx, pool, result); err != nil {
				return err
			}
			continue
		}
		if outcome == orchestration.PublishRejectedPermanent {
			if err := QuarantineWorkWakeup(ctx, pool, result); err != nil {
				return err
			}
			continue
		}
		if err := MarkWorkWakeupPublished(ctx, pool, result); err != nil {
			return err
		}
	}
	return nil
}

// RepairMissingWakeups recreates only a lost or aged-out wakeup hint. It locks
// the semantic due-work row, increments its transport generation, and inserts
// exactly one matching typed outbox intent before committing. A live outbox
// claim is never stolen and command_id is never changed.
func RepairMissingWakeups(ctx context.Context, pool *pgxpool.Pool, req RepairWakeupRequest) ([]string, error) {
	if pool == nil || req.Now.IsZero() || req.DeliveryGrace <= 0 || req.Limit <= 0 || req.Limit > 100 || req.MaxGeneration < 0 {
		return nil, fmt.Errorf("invalid wakeup repair request")
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return nil, fmt.Errorf("begin wakeup repair: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	rows, err := tx.Query(ctx, `SELECT d.id::text,d.command_id::text,d.subject_type,d.subject_id::text,d.work_type,d.expected_revision,d.delivery_generation
FROM due_work d LEFT JOIN outbox_events o ON o.delivery_kind='WORK_WAKEUP' AND o.due_work_id=d.id AND o.delivery_generation=d.delivery_generation
WHERE d.delivery_state='PENDING' AND d.not_before <= $1 AND d.delivery_generation < $2
  AND (o.event_id IS NULL OR (o.delivery_state='PUBLISHED' AND o.published_at <= $3))
ORDER BY d.not_before,d.created_at,d.id LIMIT $4 FOR UPDATE OF d SKIP LOCKED`, req.Now, req.MaxGeneration, req.Now.Add(-req.DeliveryGrace), req.Limit)
	if err != nil {
		return nil, fmt.Errorf("lock repairable wakeups: %w", err)
	}
	type repairable struct {
		id, commandID, subjectType, subjectID, workType string
		revision, generation                            int64
	}
	items := []repairable{}
	for rows.Next() {
		var item repairable
		if err := rows.Scan(&item.id, &item.commandID, &item.subjectType, &item.subjectID, &item.workType, &item.revision, &item.generation); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan repairable wakeup: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, classifyOutboxError(err, "read repairable wakeups")
	}
	rows.Close()
	ids := make([]string, 0, len(items))
	for _, item := range items {
		generation := item.generation + 1
		if _, err := tx.Exec(ctx, `UPDATE due_work SET delivery_generation=$2,updated_at=$3 WHERE id=$1::uuid AND delivery_state='PENDING' AND delivery_generation=$4`, item.id, generation, req.Now, item.generation); err != nil {
			return nil, classifyOutboxError(err, "advance wakeup generation")
		}
		queueClass := queueClassForWorkType(item.workType)
		if _, err := tx.Exec(ctx, `INSERT INTO outbox_events (event_id,aggregate_type,aggregate_id,aggregate_revision,event_type,schema_version,payload,next_attempt_at,delivery_kind,due_work_id,command_id,delivery_generation,queue_class) VALUES (md5($1)::uuid,$2,$3::uuid,$4,'work.wakeup.requested',1,'{}'::jsonb,$5,'WORK_WAKEUP',$6::uuid,$7::uuid,$8,$9)`, "fleetforge.work-wakeup/v1:"+item.id+":"+fmt.Sprint(generation), item.subjectType, item.subjectID, item.revision, req.Now, item.id, item.commandID, generation, queueClass); err != nil {
			return nil, fmt.Errorf("insert repaired wakeup: %w", err)
		}
		ids = append(ids, item.id)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, classifyOutboxError(err, "commit wakeup repair")
	}
	return ids, nil
}

func queueClassForWorkType(workType string) string {
	switch {
	case strings.Contains(workType, "DISCOVER"):
		return string(orchestration.QueueDiscovery)
	case strings.Contains(workType, "ARTIFACT"):
		return string(orchestration.QueueArtifactVerification)
	case workType == "CANARY_START" || workType == "LATER_WAVE_START" || workType == "SUBMIT" || workType == "UPDATE_STEP":
		return string(orchestration.QueueUpdateStep)
	case strings.Contains(workType, "RECONCIL"):
		return string(orchestration.QueueReconciliation)
	default:
		return string(orchestration.QueueControl)
	}
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

// RepairExpiredOutbox releases only expired delivery claims. It never marks an
// event published and never invokes a publisher; a later claim may therefore
// redeliver the same event ID, which is the explicit at-least-once boundary.
func RepairExpiredOutbox(ctx context.Context, pool *pgxpool.Pool, now time.Time, limit int) ([]string, error) {
	if now.IsZero() || limit <= 0 || limit > 100 {
		return nil, fmt.Errorf("invalid outbox repair request")
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return nil, fmt.Errorf("begin outbox repair: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	rows, err := tx.Query(ctx, `SELECT event_id FROM outbox_events WHERE delivery_state='CLAIMED' AND claim_expires_at <= $1 ORDER BY claim_expires_at, created_at, event_id LIMIT $2 FOR UPDATE SKIP LOCKED`, now, limit)
	if err != nil {
		return nil, fmt.Errorf("lock expired outbox: %w", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan expired outbox: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("read expired outbox: %w", err)
	}
	rows.Close()
	for _, id := range ids {
		if _, err := tx.Exec(ctx, `UPDATE outbox_events SET delivery_state='PENDING', claim_token=NULL, claim_owner=NULL, claim_expires_at=NULL, next_attempt_at=$2 WHERE event_id=$1::uuid AND delivery_state='CLAIMED'`, id, now); err != nil {
			return nil, classifyOutboxError(err, "repair expired outbox")
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, classifyOutboxError(err, "commit outbox repair")
	}
	return ids, nil
}

func classifyOutboxError(err error, operation string) error {
	if isSerialization(err) {
		return fmt.Errorf("%w: %s", ErrOutboxClaimConflict, operation)
	}
	return fmt.Errorf("%s: %w", operation, err)
}
