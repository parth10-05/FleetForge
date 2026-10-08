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

-- A due-work insert and its initial transport hint share one database
-- transaction.  The deterministic event ID keeps retries/backfill idempotent
-- without making a provider identifier part of domain identity.
-- +goose StatementBegin
CREATE FUNCTION create_initial_work_wakeup() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    wakeup_class varchar(32);
BEGIN
    wakeup_class := CASE
        WHEN NEW.work_type LIKE '%DISCOVER%' THEN 'discovery'
        WHEN NEW.work_type LIKE '%ARTIFACT%' THEN 'artifact-verification'
        WHEN NEW.work_type IN ('SUBMIT', 'UPDATE_STEP') THEN 'update-step'
        WHEN NEW.work_type LIKE '%RECONCIL%' THEN 'reconciliation'
        ELSE 'control'
    END;

    INSERT INTO outbox_events (
        event_id, aggregate_type, aggregate_id, aggregate_revision,
        event_type, schema_version, payload, next_attempt_at,
        delivery_kind, due_work_id, command_id, delivery_generation, queue_class
    ) VALUES (
        md5('fleetforge.work-wakeup/v1:' || NEW.id::text || ':' || NEW.delivery_generation::text)::uuid,
        NEW.subject_type, NEW.subject_id, NEW.expected_revision,
        'work.wakeup.requested', 1, '{}'::jsonb, NEW.not_before,
        'WORK_WAKEUP', NEW.id, NEW.command_id, NEW.delivery_generation, wakeup_class
    ) ON CONFLICT (due_work_id, delivery_generation) WHERE delivery_kind = 'WORK_WAKEUP'
      DO NOTHING;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER due_work_initial_wakeup
    AFTER INSERT ON due_work
    FOR EACH ROW EXECUTE FUNCTION create_initial_work_wakeup();

-- Make the forward migration safe for P03 rows created before this contract.
INSERT INTO outbox_events (
    event_id, aggregate_type, aggregate_id, aggregate_revision,
    event_type, schema_version, payload, next_attempt_at,
    delivery_kind, due_work_id, command_id, delivery_generation, queue_class
)
SELECT
    md5('fleetforge.work-wakeup/v1:' || d.id::text || ':' || d.delivery_generation::text)::uuid,
    d.subject_type, d.subject_id, d.expected_revision,
    'work.wakeup.requested', 1, '{}'::jsonb, d.not_before,
    'WORK_WAKEUP', d.id, d.command_id, d.delivery_generation,
    CASE
        WHEN d.work_type LIKE '%DISCOVER%' THEN 'discovery'
        WHEN d.work_type LIKE '%ARTIFACT%' THEN 'artifact-verification'
        WHEN d.work_type IN ('SUBMIT', 'UPDATE_STEP') THEN 'update-step'
        WHEN d.work_type LIKE '%RECONCIL%' THEN 'reconciliation'
        ELSE 'control'
    END
FROM due_work d
WHERE NOT EXISTS (
    SELECT 1 FROM outbox_events o
     WHERE o.delivery_kind = 'WORK_WAKEUP'
       AND o.due_work_id = d.id
       AND o.delivery_generation = d.delivery_generation
);

CREATE TABLE wakeup_quarantine (
    id uuid PRIMARY KEY,
    deduplication_key char(64) NOT NULL UNIQUE,
    observed_schema_version varchar(64),
    observed_queue_class varchar(32),
    due_work_id uuid,
    command_id uuid,
    reason_code varchar(64) NOT NULL,
    first_observed_at timestamptz NOT NULL,
    last_observed_at timestamptz NOT NULL,
    occurrences bigint NOT NULL DEFAULT 1,
    resolution_state varchar(32) NOT NULL DEFAULT 'OPEN',
    CONSTRAINT wakeup_quarantine_reason_known CHECK (reason_code IN ('OVERSIZE', 'MALFORMED_ENCODING', 'UNKNOWN_VERSION', 'INVALID_IDENTIFIER', 'INVALID_QUEUE_CLASS', 'INCONSISTENT_FENCING', 'UNSUPPORTED_WORK_TYPE', 'PERMANENT_PUBLISH_REJECTION')),
    CONSTRAINT wakeup_quarantine_occurrences_positive CHECK (occurrences > 0),
    CONSTRAINT wakeup_quarantine_resolution_known CHECK (resolution_state IN ('OPEN', 'RESOLVED')),
    CONSTRAINT wakeup_quarantine_observation_ordered CHECK (last_observed_at >= first_observed_at)
);

-- +goose Down
SELECT 1;
