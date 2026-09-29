-- +goose Up
-- A cleanup intent is committed before any storage operation. It is scoped to
-- one expired artifact and its one durable upload grant; it is never an
-- age-based permission to delete arbitrary objects.
CREATE TABLE artifact_cleanup_intents (
    artifact_id uuid PRIMARY KEY REFERENCES firmware_artifacts(id) ON DELETE RESTRICT,
    grant_id varchar(128) NOT NULL UNIQUE,
    expected_expired_revision bigint NOT NULL CHECK (expected_expired_revision > 0),
    state varchar(16) NOT NULL DEFAULT 'PENDING' CHECK (state IN ('PENDING','COMPLETED')),
    created_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz
);

CREATE INDEX artifact_cleanup_intents_pending_index
    ON artifact_cleanup_intents(created_at, artifact_id)
    WHERE state = 'PENDING';

-- +goose Down
SELECT 1;
