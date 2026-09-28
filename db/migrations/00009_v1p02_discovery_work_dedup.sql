-- +goose Up
-- At most one active discovery intent may exist for one target configuration
-- revision. Completed/reconciliation history is intentionally not included.
CREATE UNIQUE INDEX due_work_target_discovery_active_unique
    ON due_work (subject_id, work_type, expected_revision)
 WHERE subject_type = 'TARGET'
   AND work_type = 'DISCOVER'
   AND delivery_state IN ('PENDING', 'CLAIMED');
-- +goose Down
DROP INDEX due_work_target_discovery_active_unique;
