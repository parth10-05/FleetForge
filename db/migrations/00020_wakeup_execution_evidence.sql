-- +goose Up
-- A completed safe wakeup is durably evidenced in the same transaction as its
-- fenced due-work transition. This is execution evidence, not a firmware or
-- provider-side-effect journal.
CREATE TABLE wakeup_execution_evidence (
    id uuid PRIMARY KEY,
    due_work_id uuid NOT NULL UNIQUE REFERENCES due_work(id) ON DELETE RESTRICT,
    command_id uuid NOT NULL,
    delivery_generation bigint NOT NULL,
    work_type varchar(128) NOT NULL,
    completed_at timestamptz NOT NULL,
    CONSTRAINT wakeup_execution_evidence_generation_nonnegative CHECK (delivery_generation >= 0),
    CONSTRAINT wakeup_execution_evidence_work_type_nonempty CHECK (length(work_type) > 0)
);

CREATE INDEX wakeup_execution_evidence_command_index
    ON wakeup_execution_evidence (command_id, completed_at DESC);

-- +goose Down
SELECT 1;
