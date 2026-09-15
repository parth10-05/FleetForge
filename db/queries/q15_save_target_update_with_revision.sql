-- P03-Q15: save-target-update-with-revision.sql
-- Access pattern: atomic target-update save with expected-revision guard.
-- Zero rows affected → revision conflict (caller maps to conflict error).
-- name: SaveTargetUpdateWithRevision :execrows
UPDATE target_updates
   SET state = $3,
       revision = revision + 1,
       command_epoch = $4,
       resume_step = $5,
       updated_at = now()
 WHERE id = $1
   AND revision = $2;
