-- P03-Q06: check-active-operation-reservations-for-target.sql
-- Access pattern: parent release and quarantine checks find unresolved target operations.
-- Index: operation_reservations_target_active_index ON operation_reservations (target_id, state)
--        WHERE state IN ('ACTIVE', 'QUARANTINED')
SELECT id,
       admission_id,
       target_id,
       campaign_id,
       scope_level,
       scope_key,
       state,
       reserved_at,
       released_at,
       release_reason
  FROM operation_reservations
 WHERE target_id = $1
   AND state IN ('ACTIVE', 'QUARANTINED');
