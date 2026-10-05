-- P03-Q09: claim-outbox-events.sql
-- Access pattern: ordered SKIP LOCKED publisher claims for unpublished events.
-- Index: outbox_events_publishable_index ON outbox_events (next_attempt_at, created_at)
--        WHERE delivery_state = 'PENDING'
-- The caller writes a fencing token and commits the claim before publisher
-- invocation. The publisher receives the durable event_id unchanged.
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
   AND delivery_kind = 'DOMAIN_EVENT'
   AND next_attempt_at <= $1::timestamptz
 ORDER BY next_attempt_at ASC, created_at ASC
LIMIT $2::integer
   FOR UPDATE SKIP LOCKED;
