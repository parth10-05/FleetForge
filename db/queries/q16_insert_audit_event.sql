-- P03-Q16: insert-audit-event.sql
-- Access pattern: append a single audit event within a unit-of-work transaction.
-- Append-only enforced by reject_audit_event_mutation trigger.
-- name: InsertAuditEvent :exec
INSERT INTO audit_events
    (aggregate_type, aggregate_id, aggregate_revision, event_sequence,
     actor_id, command_id, correlation_id, causation_id, action, reason_code, before_hash, after_hash)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12);
