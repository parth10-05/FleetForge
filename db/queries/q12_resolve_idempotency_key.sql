-- P03-Q12: resolve-idempotency-key.sql
-- Access pattern: look up an idempotency record by actor, route, and key hash.
-- Index: idempotency_records_actor_route_key_unique UNIQUE (actor_id, route, key_hash)
SELECT id,
       actor_id,
       route,
       key_hash,
       request_hash,
       command_id,
       state,
       owner_token,
       claim_expires_at,
       completed_at,
       response_status,
       response_reference,
       created_at
  FROM idempotency_records
 WHERE actor_id = $1
   AND route = $2
   AND key_hash = $3;
