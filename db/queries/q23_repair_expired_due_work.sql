-- P03-Q23: repair-expired-due-work.sql
-- Access pattern: bounded, ordered repair candidates. The caller holds rows,
-- then moves SUBMIT work to reconciliation or safe non-submit work to pending.
-- name: RepairExpiredDueWork :many
SELECT id, work_type
  FROM due_work
 WHERE delivery_state = 'CLAIMED'
   AND lease_expires_at <= $1::timestamptz
 ORDER BY lease_expires_at ASC, created_at ASC, id ASC
LIMIT $2::integer
 FOR UPDATE SKIP LOCKED;
