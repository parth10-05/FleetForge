-- +goose Up
ALTER TABLE artifact_cleanup_intents
    DROP CONSTRAINT artifact_cleanup_intents_state_check;

ALTER TABLE artifact_cleanup_intents
    ADD COLUMN lease_token uuid,
    ADD COLUMN lease_owner varchar(128),
    ADD COLUMN lease_expires_at timestamptz,
    ADD CONSTRAINT artifact_cleanup_intents_state_check
        CHECK (state IN ('PENDING','CLAIMED','COMPLETED')),
    ADD CONSTRAINT artifact_cleanup_intents_lease_shape
        CHECK ((state = 'CLAIMED') = (lease_token IS NOT NULL AND lease_owner IS NOT NULL AND lease_expires_at IS NOT NULL));

CREATE INDEX artifact_cleanup_intents_claim_index
    ON artifact_cleanup_intents(created_at, artifact_id)
    WHERE state = 'PENDING';

-- +goose Down
SELECT 1;
