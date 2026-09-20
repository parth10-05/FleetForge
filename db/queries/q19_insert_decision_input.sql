-- P03-Q19: insert-decision-input.sql
-- Access pattern: persist the normalized reducer evidence in the same unit of
-- work as its aggregate revision, audit event, and outbox envelope.
-- name: InsertDecisionInput :exec
INSERT INTO decision_inputs
    (id, aggregate_type, aggregate_id, aggregate_revision, decision_sequence,
     logical_time, reducer_version, policy_version, normalized_facts, result, reason_code)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11);
