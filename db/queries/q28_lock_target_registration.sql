-- lock-target-registration.sql
-- Access pattern: serialize comparison of an existing target registration.
-- The repository uses this only after Q27 reported an external-ID conflict.
-- name: LockTargetRegistration :one
SELECT id, external_identity, endpoint_reference, site_key, rack_key,
       revision, target_key, contract_version, credential_reference,
       transport_policy_version, registration_hash, trust_metadata_complete
  FROM targets
 WHERE external_identity = $1
 FOR UPDATE;
