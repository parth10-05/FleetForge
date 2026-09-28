-- P02-Q29: schedule-target-discovery.sql
-- Access pattern: schedule one active read-only discovery job per target
-- configuration revision. Conflict means an active equivalent job already
-- exists and is a successful idempotent outcome.
-- name: ScheduleTargetDiscovery :one
INSERT INTO due_work
    (id, command_id, subject_type, subject_id, work_type, schema_version,
     not_before, expected_revision, expected_command_epoch)
VALUES ($1, $2, 'TARGET', $3, 'DISCOVER', 1, $4, $5, NULL)
ON CONFLICT (subject_id, work_type, expected_revision)
 WHERE subject_type = 'TARGET'
   AND work_type = 'DISCOVER'
   AND delivery_state IN ('PENDING', 'CLAIMED')
DO NOTHING
RETURNING id;
