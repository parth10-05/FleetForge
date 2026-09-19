-- P03-Q10: claim-due-work.sql
-- Access pattern: ordered SKIP LOCKED claims for due work.
-- Index: due_work_claimable_index ON due_work (not_before, created_at)
--        WHERE delivery_state = 'PENDING'
-- The caller holds these rows in one short transaction, writes the supplied
-- per-row fencing tokens, then commits before any external work begins.
-- name: ClaimDueWork :many
SELECT id,
       command_id,
       subject_type,
       subject_id,
       work_type,
       schema_version,
       not_before,
       expected_revision,
       expected_command_epoch,
       delivery_state,
       delivery_generation,
       lease_token,
       lease_owner,
       lease_expires_at,
       created_at,
       updated_at
  FROM due_work
 WHERE delivery_state = 'PENDING'
   AND not_before <= $1::timestamptz
ORDER BY not_before ASC, created_at ASC
LIMIT $2::integer
   FOR UPDATE SKIP LOCKED;
