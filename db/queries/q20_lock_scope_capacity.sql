-- P03-Q20: lock-scope-capacity.sql
-- Access pattern: admission transaction locks one durable scope counter in the
-- fixed GLOBAL -> SITE -> RACK order defined by ADR-021.
-- name: LockScopeCapacity :one
SELECT scope_level, scope_key, capacity_limit, active_reservation_count, updated_at
  FROM scope_capacities
 WHERE scope_level = $1
   AND scope_key = $2
 FOR UPDATE;
