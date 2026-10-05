-- +goose Up
-- A wakeup is a transport hint for an authoritative due_work row.  Its typed
-- linkage makes that relationship queryable without parsing an event payload.
ALTER TABLE outbox_events
    ADD COLUMN delivery_kind varchar(32) NOT NULL DEFAULT 'DOMAIN_EVENT',
    ADD COLUMN due_work_id uuid REFERENCES due_work(id) ON DELETE RESTRICT,
    ADD COLUMN command_id uuid,
    ADD COLUMN delivery_generation bigint,
    ADD COLUMN queue_class varchar(32);

ALTER TABLE outbox_events
    ADD CONSTRAINT outbox_events_delivery_kind_known
        CHECK (delivery_kind IN ('DOMAIN_EVENT', 'WORK_WAKEUP')),
    ADD CONSTRAINT outbox_events_work_wakeup_linkage
        CHECK (
            (delivery_kind = 'DOMAIN_EVENT'
                AND due_work_id IS NULL
                AND command_id IS NULL
                AND delivery_generation IS NULL
                AND queue_class IS NULL)
            OR
            (delivery_kind = 'WORK_WAKEUP'
                AND due_work_id IS NOT NULL
                AND command_id IS NOT NULL
                AND delivery_generation >= 0
                AND queue_class IN ('control', 'discovery', 'artifact-verification', 'update-step', 'reconciliation'))
        );

CREATE UNIQUE INDEX outbox_events_work_wakeup_generation_unique
    ON outbox_events (due_work_id, delivery_generation)
    WHERE delivery_kind = 'WORK_WAKEUP';

CREATE INDEX outbox_events_work_wakeup_publishable_index
    ON outbox_events (next_attempt_at, created_at, event_id)
    WHERE delivery_state = 'PENDING' AND delivery_kind = 'WORK_WAKEUP';

COMMENT ON INDEX outbox_events_work_wakeup_publishable_index IS
    'P04: oldest-first, bounded SKIP LOCKED publication of typed due-work wakeups.';

-- +goose Down
SELECT 1;
