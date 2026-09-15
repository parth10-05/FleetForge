-- P03-Q09: claim-outbox-events.sql
-- Access pattern: ordered SKIP LOCKED publisher claims for unpublished events.
-- Index: outbox_events_publishable_index ON outbox_events (next_attempt_at, created_at)
--        WHERE delivery_state = 'PENDING'
-- Caller must set delivery_state = 'CLAIMED' after this SELECT within the same transaction.
-- name: ClaimOutboxEvents :many
SELECT event_id,
       aggregate_type,
       aggregate_id,
       aggregate_revision,
       event_type,
       schema_version,
       payload,
       delivery_state,
       delivery_attempts,
       next_attempt_at,
       last_error_class,
       published_at,
       created_at
  FROM outbox_events
 WHERE delivery_state = 'PENDING'
   AND next_attempt_at <= now()
 ORDER BY next_attempt_at ASC, created_at ASC
 LIMIT $1
   FOR UPDATE SKIP LOCKED;
