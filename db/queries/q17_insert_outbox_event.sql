-- P03-Q17: insert-outbox-event.sql
-- Access pattern: insert an outbox event envelope within a unit-of-work transaction.
INSERT INTO outbox_events
    (event_id, aggregate_type, aggregate_id, aggregate_revision,
     event_type, schema_version, payload)
VALUES ($1, $2, $3, $4, $5, $6, $7);
