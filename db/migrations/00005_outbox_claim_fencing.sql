-- +goose Up
ALTER TABLE outbox_events
    ADD COLUMN claim_token uuid,
    ADD COLUMN claim_owner varchar(256),
    ADD COLUMN claim_expires_at timestamptz;

ALTER TABLE outbox_events
    ADD CONSTRAINT outbox_events_claim_consistent CHECK (
        (delivery_state = 'CLAIMED' AND claim_token IS NOT NULL AND claim_owner IS NOT NULL AND claim_expires_at IS NOT NULL)
        OR
        (delivery_state <> 'CLAIMED' AND claim_token IS NULL AND claim_owner IS NULL AND claim_expires_at IS NULL)
    );

CREATE INDEX outbox_events_expired_claim_index ON outbox_events (claim_expires_at, created_at) WHERE delivery_state = 'CLAIMED';
COMMENT ON INDEX outbox_events_expired_claim_index IS 'P03-Q24: identify expired fenced outbox claims for bounded retry.';

-- +goose Down
SELECT 1;
