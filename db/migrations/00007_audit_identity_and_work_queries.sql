-- +goose Up
-- Audit correlation and causation are explicit immutable identities. Existing
-- rows can only be upgraded when their already-durable command identity is
-- present; a migration must fail rather than invent causality.
ALTER TABLE audit_events
    ADD COLUMN correlation_id uuid,
    ADD COLUMN causation_id uuid;

UPDATE audit_events
   SET correlation_id = command_id,
       causation_id = command_id
 WHERE command_id IS NOT NULL
   AND (correlation_id IS NULL OR causation_id IS NULL);

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM audit_events WHERE correlation_id IS NULL OR causation_id IS NULL) THEN
        RAISE EXCEPTION 'cannot upgrade audit_events without durable command identity' USING ERRCODE = '23514';
    END IF;
END;
$$;
-- +goose StatementEnd

ALTER TABLE audit_events
    ALTER COLUMN correlation_id SET NOT NULL,
    ALTER COLUMN causation_id SET NOT NULL;

CREATE INDEX due_work_oldest_pending_index ON due_work (not_before, created_at, id)
    WHERE delivery_state = 'PENDING';
COMMENT ON INDEX due_work_oldest_pending_index IS 'P03-Q25: inspect oldest due pending work without scanning completed history.';

CREATE INDEX outbox_events_oldest_pending_index ON outbox_events (next_attempt_at, created_at, event_id)
    WHERE delivery_state = 'PENDING';
COMMENT ON INDEX outbox_events_oldest_pending_index IS 'P03-Q26: inspect oldest publishable outbox event without scanning published history.';

-- +goose Down
SELECT 1;
