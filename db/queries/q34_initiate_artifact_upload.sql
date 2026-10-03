-- initiate-artifact-upload.sql
-- Access pattern: reserve one immutable artifact declaration before a scoped
-- direct-upload grant is issued. The display name is deliberately not stored
-- as an object key.
-- name: InitiateArtifactUpload :one
INSERT INTO firmware_artifacts (id, firmware_version, sha256_digest, object_reference, upload_grant_id, state)
VALUES ($1, $2, $3, $4, $5, 'UPLOADING')
ON CONFLICT (id) DO NOTHING
RETURNING id;
