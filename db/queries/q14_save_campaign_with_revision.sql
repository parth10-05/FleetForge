-- P03-Q14: save-campaign-with-revision.sql
-- Access pattern: atomic campaign save with expected-revision guard.
-- Zero rows affected → revision conflict (caller maps to conflict error).
-- name: SaveCampaignWithRevision :execrows
UPDATE campaigns
   SET state = $3,
       revision = revision + 1,
       command_epoch = $4,
       suspended_phase = $5,
       policy_version_id = $6,
       frozen_policy_hash = $7,
       frozen_target_snapshot_hash = $8,
       updated_at = now()
 WHERE id = $1
   AND revision = $2;
