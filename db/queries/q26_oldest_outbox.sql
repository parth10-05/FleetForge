-- P03-Q26: oldest-outbox.sql
-- Access pattern: inspect oldest publishable outbox rows for planning and
-- query-plan evidence without scanning published history.
-- name: OldestOutbox :many
SELECT event_id, next_attempt_at, created_at
  FROM outbox_events
 WHERE delivery_state = 'PENDING'
 ORDER BY next_attempt_at ASC, created_at ASC, event_id ASC
 LIMIT $1::integer;
