-- +goose Up
-- Authenticated event callbacks are advisory receipts. They are retained for
-- auditability but never authorize a target-state transition or submission.
CREATE TABLE execution_event_receipts (
    id uuid PRIMARY KEY,
    source varchar(32) NOT NULL,
    event_id varchar(128) NOT NULL,
    target_key varchar(128) NOT NULL,
    task_id varchar(256) NOT NULL,
    sequence bigint NOT NULL,
    observed_at timestamptz NOT NULL,
    received_at timestamptz NOT NULL,
    payload_digest varchar(71) NOT NULL,
    disposition varchar(32) NOT NULL,
    target_update_id uuid REFERENCES target_updates(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT execution_event_receipts_source_event_unique UNIQUE (source,event_id),
    CONSTRAINT execution_event_receipts_values CHECK (
        source='SYNTHETIC_REDFISH_V1' AND length(event_id)>0 AND length(target_key)>0
        AND length(task_id)>0 AND sequence>0 AND observed_at<=received_at
        AND payload_digest ~ '^sha256:[0-9a-f]{64}$'
        AND disposition IN ('SCHEDULED','DUPLICATE','LATE','UNMATCHED','QUARANTINED')
    )
);

CREATE TABLE execution_event_heads (
    target_key varchar(128) PRIMARY KEY,
    sequence bigint NOT NULL CHECK (sequence>0),
    receipt_id uuid NOT NULL REFERENCES execution_event_receipts(id) ON DELETE RESTRICT,
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE execution_event_quarantine (
    id uuid PRIMARY KEY,
    source varchar(32) NOT NULL,
    payload_digest varchar(71) NOT NULL,
    reason_code varchar(64) NOT NULL,
    received_at timestamptz NOT NULL,
    CONSTRAINT execution_event_quarantine_values CHECK (
        source='SYNTHETIC_REDFISH_V1' AND payload_digest ~ '^sha256:[0-9a-f]{64}$'
        AND length(reason_code)>0 AND length(reason_code)<=64
    )
);

CREATE INDEX execution_event_receipts_target_sequence_index
    ON execution_event_receipts (target_key,sequence DESC);

-- +goose StatementBegin
CREATE FUNCTION fleetforge_reject_execution_event_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'FleetForge execution event evidence is append-only';
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER execution_event_receipts_immutable
    BEFORE UPDATE OR DELETE ON execution_event_receipts
    FOR EACH ROW EXECUTE FUNCTION fleetforge_reject_execution_event_mutation();

-- +goose Down
SELECT 1;
