-- +goose Up
-- CANARY_START is consumed by the update-step wakeup handler, which atomically
-- advances it to PRECHECK.  Keep the database trigger and repair path aligned.
CREATE OR REPLACE FUNCTION create_initial_work_wakeup() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    wakeup_class varchar(32);
BEGIN
    wakeup_class := CASE
        WHEN NEW.work_type LIKE '%DISCOVER%' THEN 'discovery'
        WHEN NEW.work_type LIKE '%ARTIFACT%' THEN 'artifact-verification'
        WHEN NEW.work_type IN ('CANARY_START', 'LATER_WAVE_START', 'SUBMIT', 'UPDATE_STEP') THEN 'update-step'
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

UPDATE outbox_events o
SET queue_class = 'update-step'
FROM due_work d
WHERE o.delivery_kind = 'WORK_WAKEUP'
  AND o.delivery_state = 'PENDING'
  AND o.due_work_id = d.id
  AND d.work_type IN ('CANARY_START', 'LATER_WAVE_START')
  AND o.queue_class <> 'update-step';

-- +goose Down
SELECT 1;
