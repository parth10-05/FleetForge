-- +goose Up
-- An artifact version is verified at most once while its immutable evidence is
-- still awaiting a worker result. Completed work remains durable history.
CREATE UNIQUE INDEX due_work_artifact_verification_active_unique
    ON due_work (subject_id, work_type, expected_revision)
 WHERE subject_type = 'ARTIFACT'
   AND work_type = 'VERIFY_ARTIFACT'
   AND delivery_state IN ('PENDING', 'CLAIMED');

-- +goose Down
DROP INDEX due_work_artifact_verification_active_unique;
