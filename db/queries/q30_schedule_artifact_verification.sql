-- schedule-artifact-verification.sql
-- Access pattern: schedule one active immutable-version verification intent.
-- Conflict is an idempotent replay while a worker still owns or awaits work.
-- name: ScheduleArtifactVerification :one
INSERT INTO due_work
    (id, command_id, subject_type, subject_id, work_type, schema_version,
     not_before, expected_revision, expected_command_epoch)
VALUES ($1, $2, 'ARTIFACT', $3, 'VERIFY_ARTIFACT', 1, $4, $5, NULL)
ON CONFLICT (subject_id, work_type, expected_revision)
 WHERE subject_type = 'ARTIFACT'
   AND work_type = 'VERIFY_ARTIFACT'
   AND delivery_state IN ('PENDING', 'CLAIMED')
DO NOTHING
RETURNING id;
