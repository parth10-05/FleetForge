-- P02-Q27: register-trusted-target.sql
-- Access pattern: insert a fully validated simulator registration exactly once.
-- A caller that receives no row must lock/read the existing external identity
-- and compare registration_hash; it must never silently mutate an endpoint.
-- name: RegisterTrustedTarget :one
INSERT INTO targets
    (id, external_identity, endpoint_reference, site_key, rack_key,
     administrative_state, target_key, contract_version, credential_provider,
     credential_reference, transport_policy_version, registration_hash,
     trust_metadata_complete)
VALUES
    ($1, $2, $3, $4, $5, 'ACTIVE', $6, 'synthetic-redfish-v1',
     'synthetic-test', $7, $8, $9, true)
ON CONFLICT (external_identity) DO NOTHING
RETURNING id, external_identity, endpoint_reference, site_key, rack_key,
          revision, target_key, contract_version, credential_reference,
          transport_policy_version, registration_hash;
