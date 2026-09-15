-- P03-Q11: audit-timeline.sql
-- Access pattern: reconstruct an immutable aggregate audit timeline.
-- Index: audit_events_timeline_index ON audit_events
--        (aggregate_type, aggregate_id, aggregate_revision, event_sequence)
-- name: AuditTimeline :many
SELECT id,
       aggregate_type,
       aggregate_id,
       aggregate_revision,
       event_sequence,
       actor_id,
       command_id,
       action,
       reason_code,
       before_hash,
       after_hash,
       created_at
  FROM audit_events
 WHERE aggregate_type = $1
   AND aggregate_id = $2
 ORDER BY aggregate_revision ASC, event_sequence ASC;
