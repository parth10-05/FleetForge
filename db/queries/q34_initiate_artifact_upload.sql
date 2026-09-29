-- P02-Q34: initiate-artifact-upload.sql
-- Access pattern: reserve one immutable artifact declaration before a scoped
-- direct-upload grant is issued. The display name is deliberately not stored
-- as an object key.
-- name: InitiateArtifactUpload :one
INSERT INTO firmware_artifacts (id, firmware_version, sha256_digest, object_reference, state)
VALUES ($1, $2, $3, $4, 'UPLOADING')
ON CONFLICT (id) DO NOTHING
RETURNING id;
