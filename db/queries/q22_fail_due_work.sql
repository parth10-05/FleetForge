-- P03-Q22: fail-due-work.sql
-- Access pattern: fenced non-success result commit. A worker can only return
-- pending work while it still owns the token and captured aggregate fence.
-- name: FailDueWork :execrows
UPDATE due_work
   SET delivery_state = 'PENDING',
       not_before = $5::timestamptz,
       lease_token = NULL,
       lease_owner = NULL,
       lease_expires_at = NULL,
       updated_at = $6::timestamptz
WHERE id = $1::uuid
   AND delivery_state = 'CLAIMED'
   AND lease_token = $2::uuid
   AND expected_revision = $3
   AND expected_command_epoch IS NOT DISTINCT FROM $4::bigint;
