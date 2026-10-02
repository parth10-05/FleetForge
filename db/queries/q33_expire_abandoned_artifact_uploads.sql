-- P02-Q33: expire-abandoned-artifact-uploads.sql
-- Access pattern: bounded cleanup candidates. A row with a bound immutable
-- object version is never eligible, regardless of age.
-- name: ExpireAbandonedArtifactUploads :many
WITH candidates AS (
    SELECT a.id
      FROM firmware_artifacts a
     WHERE a.state = 'UPLOADING'
       AND a.created_at <= $1::timestamptz
       AND a.upload_grant_id IS NOT NULL
       AND NOT EXISTS (
           SELECT 1 FROM artifact_object_versions object_version
            WHERE object_version.artifact_id = a.id
       )
       AND NOT EXISTS (
           SELECT 1 FROM artifact_holds hold
            WHERE hold.artifact_id = a.id AND hold.released_at IS NULL
       )
     ORDER BY a.created_at ASC, a.id ASC
     LIMIT $2
     FOR UPDATE SKIP LOCKED
), expired AS (
UPDATE firmware_artifacts artifact
   SET state = 'EXPIRED', revision = artifact.revision + 1, updated_at = now()
  FROM candidates
 WHERE artifact.id = candidates.id
 RETURNING artifact.id, artifact.upload_grant_id, artifact.revision
)
INSERT INTO artifact_cleanup_intents (artifact_id, grant_id, expected_expired_revision)
SELECT id, upload_grant_id, revision
  FROM expired
ON CONFLICT (artifact_id) DO NOTHING
RETURNING artifact_id, grant_id;
