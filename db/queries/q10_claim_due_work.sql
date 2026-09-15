-- P03-Q10: claim-due-work.sql
-- Access pattern: ordered SKIP LOCKED claims for due work.
-- Index: due_work_claimable_index ON due_work (not_before, created_at)
--        WHERE delivery_state = 'PENDING'
-- Caller must UPDATE delivery_state = 'CLAIMED' + set lease fields after this SELECT.
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
   AND not_before <= now()
 ORDER BY not_before ASC, created_at ASC
 LIMIT $1
   FOR UPDATE SKIP LOCKED;
