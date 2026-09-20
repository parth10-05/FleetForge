-- P03-Q21: complete-due-work.sql
-- Access pattern: fenced result commit. Zero rows means stale ownership,
-- aggregate revision, or command epoch and must fail closed.
-- name: CompleteDueWork :execrows
UPDATE due_work
   SET delivery_state = 'COMPLETED',
       lease_token = NULL,
       lease_owner = NULL,
       lease_expires_at = NULL,
       updated_at = $5::timestamptz
WHERE id = $1::uuid
   AND delivery_state = 'CLAIMED'
   AND lease_token = $2::uuid
   AND expected_revision = $3
   AND expected_command_epoch IS NOT DISTINCT FROM $4::bigint;
