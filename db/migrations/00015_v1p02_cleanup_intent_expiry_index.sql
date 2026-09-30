-- +goose Up
-- Bounded lease repair reads only expired CLAIMED cleanup intents.
CREATE INDEX artifact_cleanup_intents_expired_claim_index
    ON artifact_cleanup_intents(lease_expires_at, created_at, artifact_id)
    WHERE state = 'CLAIMED';

-- +goose Down
DROP INDEX artifact_cleanup_intents_expired_claim_index;
