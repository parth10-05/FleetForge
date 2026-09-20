-- P03-Q13: load-decision-inputs-for-replay.sql
-- Access pattern: reconstruct decisions after observations expire (replay support).
-- Index: decision_inputs_aggregate_revision_sequence_unique UNIQUE
--        (aggregate_type, aggregate_id, aggregate_revision, decision_sequence)
-- name: LoadDecisionInputsForReplay :many
SELECT id,
       aggregate_type,
       aggregate_id,
       aggregate_revision,
       decision_sequence,
       logical_time,
       reducer_version,
       policy_version,
       normalized_facts,
       result,
       reason_code,
       created_at
  FROM decision_inputs
 WHERE aggregate_type = $1
   AND aggregate_id = $2
 ORDER BY aggregate_revision ASC, decision_sequence ASC;
