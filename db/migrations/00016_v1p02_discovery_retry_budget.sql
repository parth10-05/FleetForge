-- +goose Up
-- P02 discovery has a bounded three-attempt read budget. It is intentionally
-- separate from generic due work so firmware/effect work retains its own
-- recovery policy.
ALTER TABLE due_work
    ADD COLUMN discovery_attempts integer NOT NULL DEFAULT 0,
    ADD CONSTRAINT due_work_discovery_attempts_nonnegative CHECK (discovery_attempts >= 0);

CREATE INDEX due_work_discovery_retry_index
    ON due_work (not_before, created_at, id)
    WHERE delivery_state = 'PENDING'
      AND subject_type = 'TARGET'
      AND work_type = 'DISCOVER'
      AND discovery_attempts < 3;

-- +goose Down
DROP INDEX due_work_discovery_retry_index;
ALTER TABLE due_work DROP CONSTRAINT due_work_discovery_attempts_nonnegative;
ALTER TABLE due_work DROP COLUMN discovery_attempts;
