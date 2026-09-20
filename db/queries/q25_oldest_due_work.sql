-- P03-Q25: oldest-due-work.sql
-- Access pattern: inspect oldest claimable due-work rows for planning and
-- query-plan evidence without scanning completed history.
-- name: OldestDueWork :many
SELECT id, not_before, created_at
  FROM due_work
 WHERE delivery_state = 'PENDING'
 ORDER BY not_before ASC, created_at ASC, id ASC
 LIMIT $1::integer;
