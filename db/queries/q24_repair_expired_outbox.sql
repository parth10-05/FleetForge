-- P03-Q24: repair-expired-outbox.sql
-- Access pattern: bounded, ordered expired outbox claims for safe retry by a
-- future publisher repairer. This query does not publish or mutate rows.
-- name: RepairExpiredOutbox :many
SELECT event_id, claim_token, claim_owner, claim_expires_at
  FROM outbox_events
 WHERE delivery_state = 'CLAIMED'
   AND claim_expires_at <= $1::timestamptz
 ORDER BY claim_expires_at ASC, created_at ASC, event_id ASC
 LIMIT $2::integer
 FOR UPDATE SKIP LOCKED;
